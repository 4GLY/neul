package cli

import (
	"os/exec"
	"runtime"
)

var openApprovalURL = defaultOpenApprovalURL

func defaultOpenApprovalURL(approvalURL string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", approvalURL)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", approvalURL)
	default:
		command = exec.Command("xdg-open", approvalURL)
	}
	if err := command.Start(); err != nil {
		return err
	}
	go func() {
		_ = command.Wait()
	}()
	return nil
}
