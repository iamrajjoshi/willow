package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"log"

	"github.com/getsentry/sentry-go"
	"github.com/iamrajjoshi/willow/internal/claude"
	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/notify"
	"github.com/iamrajjoshi/willow/internal/worktree"
	"github.com/urfave/cli/v3"
)

func pidFilePath() string {
	return filepath.Join(config.WillowHome(), "notify.pid")
}

func logFilePath() string {
	return filepath.Join(config.WillowHome(), "notify.log")
}

func notifyCmd() *cli.Command {
	return &cli.Command{
		Name:    "notify",
		Aliases: []string{"notif"},
		Usage:   "Desktop notifications for agent status changes",
		Commands: []*cli.Command{
			notifyOnCmd(),
			notifyOffCmd(),
			notifyStatusCmd(),
			notifyLogCmd(),
			notifyRunCmd(),
		},
	}
}

func notifyOnCmd() *cli.Command {
	return &cli.Command{
		Name:  "on",
		Usage: "Start the notification daemon in the background",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:  "interval",
				Usage: "Poll interval in seconds",
				Value: 3,
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			flags := parseFlags(cmd)
			u := flags.NewUI()

			// Check if already running
			if pid, ok := readPidFile(); ok {
				if isProcessRunning(pid) {
					u.Info(fmt.Sprintf("Notification daemon already running (pid %d)", pid))
					return nil
				}
				// Stale pid file
				os.Remove(pidFilePath())
			}

			// Launch the daemon as a background process
			self, err := os.Executable()
			if err != nil {
				return fmt.Errorf("failed to resolve executable: %w", err)
			}

			interval := cmd.Int("interval")
			daemon := exec.Command(self, "notify", "run", "--interval", strconv.Itoa(int(interval)))

			logFile, err := os.OpenFile(logFilePath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
			if err != nil {
				return fmt.Errorf("failed to open log file: %w", err)
			}
			daemon.Stdout = logFile
			daemon.Stderr = logFile
			daemon.Stdin = nil
			daemon.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

			if err := daemon.Start(); err != nil {
				logFile.Close()
				return fmt.Errorf("failed to start daemon: %w", err)
			}
			logFile.Close()

			// Write PID file
			if err := os.WriteFile(pidFilePath(), []byte(strconv.Itoa(daemon.Process.Pid)), 0o644); err != nil {
				return fmt.Errorf("failed to write pid file: %w", err)
			}

			// Detach — don't wait for the child
			daemon.Process.Release()

			u.Success(fmt.Sprintf("Notification daemon started (pid %d)", daemon.Process.Pid))
			return nil
		},
	}
}

func notifyOffCmd() *cli.Command {
	return &cli.Command{
		Name:  "off",
		Usage: "Stop the notification daemon",
		Action: func(_ context.Context, cmd *cli.Command) error {
			flags := parseFlags(cmd)
			u := flags.NewUI()

			pid, ok := readPidFile()
			if !ok {
				u.Info("Notification daemon is not running")
				return nil
			}

			process, err := os.FindProcess(pid)
			if err != nil {
				os.Remove(pidFilePath())
				u.Info("Notification daemon is not running")
				return nil
			}

			if err := process.Signal(syscall.SIGTERM); err != nil {
				os.Remove(pidFilePath())
				u.Info("Notification daemon is not running")
				return nil
			}

			os.Remove(pidFilePath())
			u.Success("Notification daemon stopped")
			return nil
		},
	}
}

func notifyStatusCmd() *cli.Command {
	return &cli.Command{
		Name:  "status",
		Usage: "Check if the notification daemon is running",
		Action: func(_ context.Context, cmd *cli.Command) error {
			flags := parseFlags(cmd)
			u := flags.NewUI()

			pid, ok := readPidFile()
			if !ok {
				u.Info("Notification daemon is not running")
				return nil
			}

			if isProcessRunning(pid) {
				u.Success(fmt.Sprintf("Notification daemon running (pid %d)", pid))
			} else {
				os.Remove(pidFilePath())
				u.Info("Notification daemon is not running (stale pid file cleaned)")
			}
			return nil
		},
	}
}

func notifyLogCmd() *cli.Command {
	return &cli.Command{
		Name:  "log",
		Usage: "Show notification daemon logs",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "follow",
				Aliases: []string{"f"},
				Usage:   "Follow log output (tail -f)",
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			lp := logFilePath()
			if _, err := os.Stat(lp); os.IsNotExist(err) {
				fmt.Println("No log file yet. Start the daemon with: ww notify on")
				return nil
			}
			tailCmd := "tail -50"
			if cmd.Bool("follow") {
				tailCmd = "tail -50 -f"
			}
			c := exec.Command("sh", "-c", tailCmd+" "+lp)
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			return c.Run()
		},
	}
}

// notifyRunCmd is the actual long-running daemon (called by `notify on`).
func notifyRunCmd() *cli.Command {
	return &cli.Command{
		Name:   "run",
		Usage:  "Run the notification loop (internal, use 'notify on' instead)",
		Hidden: true,
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:  "interval",
				Usage: "Poll interval in seconds",
				Value: 3,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			interval := time.Duration(cmd.Int("interval")) * time.Second

			logger := log.New(os.Stderr, "", log.LstdFlags)

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			// Clean up pid file on exit
			defer os.Remove(pidFilePath())

			cfg := config.Load("")

			logger.Printf("daemon started (interval=%s)", interval)

			check := func() {
				statuses := collectStatuses()
				transitions := claude.DetectTransitions(statuses, claude.NotifyStateFile())
				sendDesktopNotifications(logger, transitions, cfg)
			}

			check()

			for {
				select {
				case <-ctx.Done():
					logger.Println("daemon stopping (context cancelled)")
					return nil
				case <-sigCh:
					logger.Println("daemon stopping (signal)")
					return nil
				case <-ticker.C:
					check()
				}
			}
		},
	}
}

func collectStatuses() map[string]claude.Status {
	repos, err := config.ListRepos()
	if err != nil {
		return nil
	}

	statuses := make(map[string]claude.Status)
	for _, repoName := range repos {
		bareDir, err := config.ResolveRepo(repoName)
		if err != nil {
			continue
		}
		repoGit := &git.Git{Dir: bareDir}
		wts, err := worktree.List(repoGit)
		if err != nil {
			continue
		}
		for _, wt := range wts {
			if wt.IsBare {
				continue
			}
			wtDir := filepath.Base(wt.Path)
			sessions := claude.ReadAllSessions(repoName, wtDir)
			ws := claude.AggregateStatus(sessions)
			statuses[repoName+"/"+wtDir] = ws.Status
		}
	}
	return statuses
}

func sendDesktopNotifications(logger *log.Logger, transitions []claude.Transition, cfg *config.Config) {
	for _, t := range transitions {
		title := "willow"
		var body string
		switch t.ToStatus {
		case claude.StatusDone:
			body = fmt.Sprintf("\u2705 %s finished", t.Key)
		case claude.StatusWait:
			body = fmt.Sprintf("\u23F3 %s needs input", t.Key)
		default:
			continue
		}

		var err error
		if cfg.Notify.Command != "" {
			err = notify.SendCustom(cfg.Notify.Command, title, body)
		} else {
			err = notify.Send(title, body)
		}

		if err != nil {
			sentry.CaptureException(err)
			if logger != nil {
				logger.Printf("notification failed for %s: %v", t.Key, err)
			}
		} else if logger != nil {
			logger.Printf("notified: %s", body)
		}
	}
}

func readPidFile() (int, bool) {
	data, err := os.ReadFile(pidFilePath())
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, false
	}
	return pid, true
}

func isProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 checks if the process exists without actually signaling it
	return process.Signal(syscall.Signal(0)) == nil
}
