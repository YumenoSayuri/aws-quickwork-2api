package wui

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

type step string

const (
	stepOpening step = "opening"
	stepWaiting step = "waiting"
	stepDone    step = "done"
	stepFailed  step = "failed"
)

// Login exposes one interactive `quickproxy login -manual` run to the browser.
type Login struct {
	ID        string   `json:"id"`
	Provider  string   `json:"provider"`
	Stage     string   `json:"stage"`
	Step      string   `json:"step"`
	LoginURL  string   `json:"login_url"`
	Message   string   `json:"message"`
	StageDone string   `json:"stage_done"`
	AccountID int64    `json:"account_id"`
	StartedAt int64    `json:"started_at"`
	EndedAt   int64    `json:"ended_at"`
	Logs      []string `json:"logs"`

	mu       sync.Mutex
	cmd      *exec.Cmd
	stdin    *bufio.Writer
	waitErr  error
	exitedAt int64
	pending  string // authorization callback that arrived before discovery completed
}

var (
	discoveryURLRe = regexp.MustCompile(`https://[^\s]*?quick\.aws\.com/sn\?\S+`)
	accountIDRe    = regexp.MustCompile(`account (\d+)\b`)
)

func randomID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte(fmt.Sprint(time.Now().UnixNano())))
	}
	return hex.EncodeToString(buf)
}

func (l *Login) append(line string) {
	line = strings.TrimRight(strings.TrimSpace(line), "\r")
	if line == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Logs = append(l.Logs, line)
	if len(l.Logs) > 300 {
		l.Logs = l.Logs[len(l.Logs)-300:]
	}
}

func (l *Login) snapshot() *Login {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := &Login{
		ID: l.ID, Provider: l.Provider, Stage: l.Stage, Step: l.Step,
		LoginURL: l.LoginURL, Message: l.Message, AccountID: l.AccountID,
		StageDone: l.StageDone, StartedAt: l.StartedAt, EndedAt: l.EndedAt,
		Logs: append([]string(nil), l.Logs...),
	}
	return out
}

func (l *Login) setStep(s step, msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Step = string(s)
	if msg != "" {
		l.Message = msg
	}
	if (s == stepDone || s == stepFailed) && l.EndedAt == 0 {
		l.EndedAt = time.Now().Unix()
	}
}

func (l *Login) finished() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.Step == string(stepDone) || l.Step == string(stepFailed)
}

// markDiscovery records the first-stage URL printed by the CLI.
func (l *Login) markDiscovery(u string) {
	l.mu.Lock()
	fresh := l.LoginURL == ""
	if fresh {
		l.LoginURL = u
		l.StageDone = stageDiscovery
	}
	l.mu.Unlock()
	if fresh {
		l.setStep(stepWaiting, "第 2 步：打开登录地址，完成注册/登录后把地址栏 URL 贴回来")
	}
}

// markDiscoveryDone flips the stage without changing the displayed URL.
func (l *Login) markDiscoveryDone() {
	l.mu.Lock()
	l.StageDone = stageDiscovery
	pending := l.pending
	l.pending = ""
	l.mu.Unlock()
	if pending != "" {
		_ = l.submit(pending)
	}
}

// markAuthorize swaps in the second-stage authorize URL.
func (l *Login) markAuthorize(u string) {
	l.mu.Lock()
	l.LoginURL = u
	l.StageDone = stageDiscovery
	l.mu.Unlock()
	l.setStep(stepWaiting, "第 2 步：点“继续登录”完成授权，再把地址栏 URL 贴回来")
}

// pendingURL exposes the banked authorization callback, if any.
func (l *Login) pendingURL() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.pending
}

// startLogin launches the CLI in manual mode and pumps its output back into
// the record so the browser can follow the flow.
func (w *Server) startLogin(provider, stage string, timeout time.Duration) (*Login, error) {
	if provider == "" {
		provider = w.cfg.OAuth.Provider
	}
	if stage == "" {
		stage = w.cfg.OAuth.BuildStage
	}
	if timeout < 2*time.Minute {
		timeout = w.cfg.OAuth.Timeout.Duration()
	}

	cmd := exec.Command(w.binary, "login",
		"-config", w.configPath,
		"-manual",
		"-provider", provider,
		"-stage", stage,
		"-timeout", timeout.String(),
	)
	cmd.Dir = w.workDir
	cmd.Env = append(cmd.Environ(), "NO_COLOR=1")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start login process: %w", err)
	}

	l := &Login{
		ID:        randomID(),
		Provider:  provider,
		Stage:     stage,
		Step:      string(stepOpening),
		StartedAt: time.Now().Unix(),
		cmd:       cmd,
		stdin:     bufio.NewWriter(stdin),
	}

	w.mu.Lock()
	previous := w.login
	w.login = l
	w.mu.Unlock()
	if previous != nil {
		previous.stop()
	}

	var wg sync.WaitGroup
	pump := func(rd interface{ Read([]byte) (int, error) }) {
		defer wg.Done()
		sc := bufio.NewScanner(rd)
		sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for sc.Scan() {
			line := sc.Text()
			l.append(line)
			l.observe(line)
		}
	}
	wg.Add(2)
	go pump(stdout)
	go pump(stderr)

	go func() {
		wg.Wait()
		waitErr := cmd.Wait()
		l.mu.Lock()
		l.waitErr = waitErr
		l.exitedAt = time.Now().Unix()
		l.mu.Unlock()
		if waitErr != nil && !l.finished() {
			l.setStep(stepFailed, "login process exited: "+waitErr.Error())
			return
		}
		if !l.finished() {
			l.setStep(stepFailed, "login process ended without saving an account")
		}
	}()

	// The CLI prints the discovery URL as soon as it boots; give it a moment so
	// the caller gets a usable link in the first response.
	startup := time.Now().Add(12 * time.Second)
	for time.Now().Before(startup) {
		snap := l.snapshot()
		if snap.LoginURL != "" || snap.Step == string(stepFailed) || snap.Step == string(stepDone) {
			break
		}
		time.Sleep(60 * time.Millisecond)
	}

	return l.snapshot(), nil
}

// settle waits up to d for a login that already received its callback to
// reach a terminal state, so the browser sees the real outcome.
func (l *Login) settle(d time.Duration) {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if l.finished() {
			return
		}
		time.Sleep(80 * time.Millisecond)
	}
}

const (
	stageDiscovery   = "discovery"
	stageAuthorizing = "authorizing"
)

var (
	authorizeURLRe = regexp.MustCompile(`https://[^\s]+/authorize\?\S+`)
)

// observe reacts to CLI output lines.
func (l *Login) observe(line string) {
	if strings.Contains(line, "Discovery complete") {
		// The authorize URL is printed either on this line or the next one.
		if m := authorizeURLRe.FindString(line); m != "" {
			l.markAuthorize(m)
		}
		l.markDiscoveryDone()
		return
	}
	if m := authorizeURLRe.FindString(line); m != "" {
		l.markAuthorize(m)
		return
	}
	if strings.HasPrefix(line, "Then paste the callback URL containing") {
		l.markDiscoveryDone()
		return
	}
	if u := discoveryURLRe.FindString(line); u != "" {
		l.markDiscovery(u)
		return
	}
	if m := accountIDRe.FindStringSubmatch(line); m != nil && strings.Contains(line, "account") {
		var id int64
		fmt.Sscanf(m[1], "%d", &id)
		l.mu.Lock()
		l.AccountID = id
		l.mu.Unlock()
		l.setStep(stepDone, "账号已加入号池")
		return
	}
	if strings.Contains(line, "[login] failed") {
		msg := line[strings.Index(line, "[login] failed"):]
		l.setStep(stepFailed, strings.Replace(msg, "[login] failed:", "", 1))
		return
	}
	// Non-fatal rejections: the CLI keeps waiting for another URL, so surface
	// the reason instead of leaving the panel on "submitted".
	for _, prefix := range []string{
		"discovery failed:", "login failed:", "invalid URL:",
		"URL not recognized.", "This is an authorize URL",
		"This is the discovery URL.", "This discovery callback was already processed",
	} {
		if strings.Contains(line, prefix) {
			l.setStep(stepWaiting, strings.TrimSpace(strings.TrimPrefix(line, "callback URL: ")))
			return
		}
	}
}

// callbackKind mirrors the CLI's own classification of pasted URLs.
type callbackKind int

const (
	cbUnknown callbackKind = iota
	cbDiscovery
	cbAuthorize
	cbCode
)

func classify(raw string) callbackKind {
	u, err := url.Parse(raw)
	if err != nil {
		return cbUnknown
	}
	q := u.Query()
	if frag := strings.TrimPrefix(u.Fragment, "/"); frag != "" {
		if extra, err := url.ParseQuery(strings.ReplaceAll(strings.TrimPrefix(frag, "?"), "?", "&")); err == nil {
			for k, v := range extra {
				if _, exists := q[k]; !exists {
					q[k] = v
				}
			}
		}
	}
	switch {
	case q.Get("code") != "":
		return cbCode
	case q.Get("nonce") != "":
		return cbDiscovery
	case q.Get("qwSurface") != "":
		return cbUnknown // the discovery URL itself, not a callback
	case q.Get("client_id") != "" && (q.Get("response_type") == "code" || q.Get("code_challenge") != ""):
		return cbAuthorize
	case q.Get("error") != "":
		return cbCode
	}
	return cbUnknown
}

// submit forwards one pasted callback URL to the running login process.
func (l *Login) submit(raw string) error {
	raw = strings.TrimSpace(strings.Trim(strings.TrimSpace(raw), "\"'<>`"))
	raw = strings.ReplaceAll(raw, "&amp;", "&")
	if raw == "" {
		return fmt.Errorf("回调 URL 为空")
	}
	if !strings.Contains(raw, "=") {
		return fmt.Errorf("这看起来不是回调 URL")
	}
	if l.finished() {
		return fmt.Errorf("这次登录已经结束，请重新开始")
	}

	kind := classify(raw)
	if kind == cbUnknown {
		return fmt.Errorf("无法识别这个 URL：需要 discovery 回调（含 nonce=）或授权回调（含 code= 和 state=）")
	}

	l.mu.Lock()
	stageDone := l.StageDone
	l.mu.Unlock()

	if kind == cbCode {
		if stageDone != stageDiscovery {
			l.mu.Lock()
			l.pending = raw
			l.mu.Unlock()
			l.append("> " + summarise(raw))
			l.setStep(stepWaiting, "已记住授权回调，等 discovery 完成后会自动提交")
			return nil
		}
	} else if kind == cbAuthorize {
		return fmt.Errorf("这是授权地址，不是回调：请在浏览器里打开它完成登录，然后贴地址栏里含 code= 的 URL")
	}

	l.mu.Lock()
	writer := l.stdin
	l.mu.Unlock()
	if writer == nil {
		return fmt.Errorf("登录进程不接受输入")
	}
	if _, err := writer.WriteString(raw + "\n"); err != nil {
		return err
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	l.append("> " + summarise(raw))
	if kind == cbCode {
		l.setStep(stepWaiting, "已提交授权码，等待上游换取 token")
	} else {
		l.setStep(stepWaiting, "已提交 discovery 回调，等待上游返回授权地址")
	}
	return nil
}

func (l *Login) stop() {
	l.mu.Lock()
	cmd := l.cmd
	l.mu.Unlock()
	if cmd == nil || cmd.Process == nil || l.finished() {
		return
	}
	_ = cmd.Process.Kill()
	l.setStep(stepFailed, "登录已取消")
}

func summarise(raw string) string {
	if len(raw) <= 120 {
		return raw
	}
	return raw[:117] + "..."
}
