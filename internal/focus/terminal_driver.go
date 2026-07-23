package focus

import (
	"errors"
	"strings"
)

var (
	errTerminalNotFound             = errors.New("terminal not found")
	errTerminalOperationUnsupported = errors.New("terminal operation unsupported")
)

type terminalClientTarget struct {
	tty            string
	title          string
	termName       string
	currentPath    string
	capturedBundle string
}

type terminalDriver interface {
	bundleID() string
	focusClientExact(terminalClientTarget) error
	focusTitle(string) error
	openAttachment(string) error
}

type terminalClientCompatibilityDriver interface {
	focusClientCompatibility(terminalClientTarget) error
}

type terminalClientProbe interface {
	bundleID() string
	focusClient(terminalClientTarget) error
}

type exactTerminalClientProbe struct {
	terminalDriver
}

func (p exactTerminalClientProbe) focusClient(target terminalClientTarget) error {
	return p.focusClientExact(target)
}

type compatibilityTerminalClientProbe struct {
	terminalDriver
	compatibility terminalClientCompatibilityDriver
}

func (p compatibilityTerminalClientProbe) focusClient(target terminalClientTarget) error {
	return p.compatibility.focusClientCompatibility(target)
}

type appleScriptTerminalDriver struct {
	id           string
	clientScript func(terminalClientTarget) string
	titleScript  func(string) string
	openScript   func(string) string
}

func (d appleScriptTerminalDriver) bundleID() string {
	return d.id
}

func (d appleScriptTerminalDriver) focusClientExact(target terminalClientTarget) error {
	if d.clientScript == nil {
		return errTerminalOperationUnsupported
	}
	return runSelectionScript(d.clientScript(target))
}

func (d appleScriptTerminalDriver) focusTitle(title string) error {
	if d.titleScript == nil {
		return errTerminalOperationUnsupported
	}
	return runSelectionScript(d.titleScript(title))
}

func (d appleScriptTerminalDriver) openAttachment(command string) error {
	if d.openScript == nil {
		return errTerminalOperationUnsupported
	}
	return runCmd(osascriptPath, "-e", d.openScript(command))
}

type ghosttyTerminalDriver struct{}

func (ghosttyTerminalDriver) bundleID() string {
	return bundleGhostty
}

func (ghosttyTerminalDriver) focusClientExact(target terminalClientTarget) error {
	return runSelectionScript(ghosttySelectTTYScript(target.tty))
}

func (ghosttyTerminalDriver) focusClientCompatibility(target terminalClientTarget) error {
	// TERM is only a compatibility hint. Every registered terminal gets an exact
	// TTY probe before compatibility probes run, and the captured bundle also
	// identifies Ghostty clients that use a generic TERM value.
	if target.capturedBundle != bundleGhostty && !isGhosttyTerm(target.termName) {
		return errTerminalNotFound
	}
	if target.currentPath != "" {
		previousDirectories := ghosttyWorkingDirectories()
		marker := ghosttyMarkerPath(target.tty)
		if err := writeTerminalSequence(target.tty, terminalWorkingDirectorySequence(marker)); err == nil {
			terminalID := ghosttyTerminalIDByWorkingDirectory(marker)
			restorePath := target.currentPath
			if previousPath, ok := previousDirectories[terminalID]; ok {
				restorePath = previousPath
			}
			_ = writeTerminalSequence(target.tty, terminalWorkingDirectorySequence(restorePath))
			if terminalID != "" {
				return nil
			}
		}
	}
	return runSelectionScript(ghosttySelectScript(target.title))
}

func (ghosttyTerminalDriver) focusTitle(title string) error {
	return runSelectionScript(ghosttySelectScript(title))
}

func (ghosttyTerminalDriver) openAttachment(command string) error {
	return runCmd(osascriptPath, "-e", ghosttyOpenScript(command))
}

// Registry order is the fallback probe order for attached tmux clients.
var terminalDriverRegistry = []terminalDriver{
	appleScriptTerminalDriver{
		id: bundleITerm2,
		clientScript: func(target terminalClientTarget) string {
			return iterm2SelectTTYScript(target.tty)
		},
		titleScript: iterm2SelectScript,
		openScript:  iterm2OpenScript,
	},
	appleScriptTerminalDriver{
		id: bundleTerminal,
		clientScript: func(target terminalClientTarget) string {
			return terminalSelectTTYScript(target.tty)
		},
		titleScript: terminalSelectScript,
		openScript:  terminalOpenScript,
	},
	ghosttyTerminalDriver{},
}

func terminalDriverForBundle(bundle string) terminalDriver {
	for _, driver := range terminalDriverRegistry {
		if driver != nil && driver.bundleID() == bundle {
			return driver
		}
	}
	return nil
}

func terminalDriversForClient(preferredBundle string) []terminalClientProbe {
	drivers := make([]terminalDriver, 0, len(terminalDriverRegistry))
	seen := make(map[string]bool, len(terminalDriverRegistry))
	appendDriver := func(driver terminalDriver) {
		if driver == nil || seen[driver.bundleID()] {
			return
		}
		seen[driver.bundleID()] = true
		drivers = append(drivers, driver)
	}

	appendDriver(terminalDriverForBundle(preferredBundle))
	for _, driver := range terminalDriverRegistry {
		appendDriver(driver)
	}

	probes := make([]terminalClientProbe, 0, len(drivers)*2)
	for _, driver := range drivers {
		probes = append(probes, exactTerminalClientProbe{terminalDriver: driver})
	}
	for _, driver := range drivers {
		compatibility, ok := driver.(terminalClientCompatibilityDriver)
		if ok {
			probes = append(probes, compatibilityTerminalClientProbe{
				terminalDriver: driver,
				compatibility:  compatibility,
			})
		}
	}
	return probes
}

func runSelectionScript(script string) error {
	out, err := runCmdOutput(osascriptPath, "-e", script)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(out)) != selectedMarker {
		return errTerminalNotFound
	}
	return nil
}
