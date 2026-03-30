package main

import (
	"bufio"
	"context"
	"embed"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"web-claude/internal/auth"
	"web-claude/internal/config"
	"web-claude/internal/terminal"
)

const (
	commandName = "claude"
)

var commandArgs = []string{"-c"}

//go:embed static/*
var assets embed.FS

type app struct {
	authSessions *auth.SessionManager
	indexHTML    []byte
	loginHTML    []byte
	passwordHash string
	terminals    *terminal.Manager
}

type websocketMessage struct {
	Type    string `json:"type"`
	Data    string `json:"data,omitempty"`
	Cols    int    `json:"cols,omitempty"`
	Rows    int    `json:"rows,omitempty"`
	Running bool   `json:"running,omitempty"`
	Message string `json:"message,omitempty"`
}

func main() {
	gin.SetMode(gin.ReleaseMode)

	setPassword := flag.Bool("set-password", false, "set or update the web login password")
	flag.Parse()

	startupDir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	cfg, err := config.Load(config.DefaultPath(startupDir))
	if err != nil {
		log.Fatal(err)
	}
	if *setPassword {
		if err := configurePassword(cfg); err != nil {
			log.Fatal(err)
		}
		log.Printf("password updated in %s", config.DefaultPath(startupDir))
		return
	}
	if !cfg.AuthConfigured() {
		log.Fatalf("authentication is not configured; run `go run . -set-password` first")
	}
	if _, err := exec.LookPath(commandName); err != nil {
		log.Fatalf("%s is not available in PATH: %v", commandName, err)
	}

	addr := cfg.ListenAddr()
	if override := strings.TrimSpace(os.Getenv("WEB_CLAUDE_ADDR")); override != "" {
		addr = override
	}
	if err := cfg.ValidateListenAddr(addr); err != nil {
		log.Fatal(err)
	}
	workingDir, err := cfg.WorkingDir(startupDir)
	if err != nil {
		log.Fatal(err)
	}

	authSessions, err := auth.NewSessionManager(cfg.SessionSecret(), 7*24*time.Hour)
	if err != nil {
		log.Fatal(err)
	}

	application := &app{
		authSessions: authSessions,
		passwordHash: cfg.PasswordHash(),
		terminals:    terminal.NewManager(workingDir, commandName, commandArgs, cfg.RestartOnReconnect()),
	}

	router, err := newRouter(application)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("web-claude listening on http://%s", addr)
	log.Printf("claude working directory: %s", workingDir)
	if err := router.Run(addr); err != nil {
		log.Fatal(err)
	}
}

func newRouter(application *app) (*gin.Engine, error) {
	indexHTML, err := assets.ReadFile("static/index.html")
	if err != nil {
		return nil, err
	}
	application.indexHTML = indexHTML
	loginHTML, err := assets.ReadFile("static/login.html")
	if err != nil {
		return nil, err
	}
	application.loginHTML = loginHTML
	staticFS, err := fs.Sub(assets, "static")
	if err != nil {
		return nil, err
	}

	router := gin.New()
	router.Use(gin.Logger())
	router.Use(gin.RecoveryWithWriter(io.Discard))
	router.StaticFS("/static", http.FS(staticFS))

	router.GET("/login", application.handleLoginPage)
	router.POST("/login", application.handleLoginSubmit)
	router.GET("/logout", application.handleLogout)

	protected := router.Group("/")
	protected.Use(application.requireAuthentication)
	protected.GET("/", application.handleRoot)
	protected.GET("/app", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/")
	})
	protected.GET("/ws/terminal", application.handleTerminalWebSocket)

	return router, nil
}

func (a *app) requireAuthentication(c *gin.Context) {
	if a.authSessions.IsAuthenticated(c.Request) {
		return
	}

	if websocket.IsWebSocketUpgrade(c.Request) {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	c.Redirect(http.StatusSeeOther, "/login")
	c.Abort()
}

func (a *app) handleRoot(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", a.indexHTML)
}

func (a *app) handleLoginPage(c *gin.Context) {
	if a.authSessions.IsAuthenticated(c.Request) {
		c.Redirect(http.StatusSeeOther, "/")
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", a.loginHTML)
}

func (a *app) handleLoginSubmit(c *gin.Context) {
	if !auth.CheckPassword(a.passwordHash, c.PostForm("password")) {
		c.Redirect(http.StatusSeeOther, "/login?error=1")
		return
	}

	cookie, err := a.authSessions.NewCookie(isSecureRequest(c.Request))
	if err != nil {
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	http.SetCookie(c.Writer, cookie)
	c.Redirect(http.StatusSeeOther, "/")
}

func (a *app) handleLogout(c *gin.Context) {
	http.SetCookie(c.Writer, a.authSessions.ClearCookie(isSecureRequest(c.Request)))
	c.Redirect(http.StatusSeeOther, "/login")
}

func (a *app) handleTerminalWebSocket(c *gin.Context) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return sameOrigin(r)
		},
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(1 << 20)

	sessionID := "default"
	cols, rows := parseSize(c.Query("cols"), c.Query("rows"))
	attachment, err := a.terminals.Attach(sessionID, cols, rows)
	if err != nil {
		writeWSMessage(conn, websocketMessage{Type: "error", Message: "terminal unavailable"})
		return
	}
	defer attachment.Cancel()

	if len(attachment.Snapshot) > 0 {
		writeWSMessage(conn, websocketMessage{
			Type:    "snapshot",
			Data:    base64.StdEncoding.EncodeToString(attachment.Snapshot),
			Running: attachment.Running,
		})
	} else {
		writeWSMessage(conn, websocketMessage{Type: "status", Running: attachment.Running})
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case chunk, ok := <-attachment.Updates:
				if !ok {
					return
				}
				if err := writeWSMessage(conn, websocketMessage{
					Type: "output",
					Data: base64.StdEncoding.EncodeToString(chunk),
				}); err != nil {
					cancel()
					return
				}
			}
		}
	}()

	for {
		var message websocketMessage
		if err := conn.ReadJSON(&message); err != nil {
			return
		}
		switch message.Type {
		case "input":
			payload, err := base64.StdEncoding.DecodeString(message.Data)
			if err != nil {
				writeWSMessage(conn, websocketMessage{Type: "error", Message: "invalid input"})
				continue
			}
			safe, err := auth.SanitizeTerminalInput(payload)
			if err != nil {
				writeWSMessage(conn, websocketMessage{Type: "error", Message: "input rejected"})
				continue
			}
			if len(safe) == 0 {
				continue
			}
			if err := a.terminals.Input(sessionID, safe); err != nil {
				writeWSMessage(conn, websocketMessage{Type: "error", Message: "terminal write failed"})
			}
		case "resize":
			if err := a.terminals.Resize(sessionID, message.Cols, message.Rows); err != nil {
				writeWSMessage(conn, websocketMessage{Type: "error", Message: "resize failed"})
			}
		case "ping":
			writeWSMessage(conn, websocketMessage{Type: "pong"})
		default:
			writeWSMessage(conn, websocketMessage{Type: "error", Message: "unsupported message"})
		}
	}
}

func parseSize(colsRaw, rowsRaw string) (int, int) {
	cols, _ := strconv.Atoi(colsRaw)
	rows, _ := strconv.Atoi(rowsRaw)
	if cols < 40 {
		cols = 120
	}
	if rows < 10 {
		rows = 36
	}
	return cols, rows
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return parsed.Host == r.Host
}

func writeWSMessage(conn *websocket.Conn, message websocketMessage) error {
	if conn == nil {
		return errors.New("websocket connection is nil")
	}
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return conn.WriteJSON(message)
}

func configurePassword(cfg *config.Store) error {
	password, err := promptLine("New password: ")
	if err != nil {
		return err
	}
	confirm, err := promptLine("Confirm password: ")
	if err != nil {
		return err
	}
	if password != confirm {
		return errors.New("passwords do not match")
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	sessionSecret, err := auth.GenerateSessionSecret()
	if err != nil {
		return err
	}
	return cfg.SetAuth(hash, sessionSecret)
}

func promptLine(prompt string) (string, error) {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	value, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(value, "\r\n"), nil
}

func isSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
