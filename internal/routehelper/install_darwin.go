package routehelper

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

//go:embed route-helper.sh
var routeHelperScript string

var bundledHelperSHA256 string

const manageScript = `on run argv
    set commandLine to "/bin/bash -c"
    repeat with argument in argv
        set commandLine to commandLine & " " & quoted form of (contents of argument)
    end repeat
    try
        do shell script commandLine with administrator privileges
        return "completed"
    on error messageText number errorNumber
        if errorNumber is -128 then return "cancelled"
        error messageText number errorNumber
    end try
end run`

func NeedsInstall() bool {
	info, err := os.Lstat(socketPath(os.Getuid()))
	return err != nil || info.Mode()&os.ModeSocket == 0
}

func NeedsUpgrade() bool {
	if bundledHelperSHA256 == "" {
		return false
	}
	// The root-owned launchd configuration is readable even when the
	// shared PrivilegedHelperTools directory cannot be traversed.
	config, err := os.ReadFile("/Library/LaunchDaemons/org.hellotsnet.app.routes.plist")
	return err != nil || !strings.Contains(string(config), "<string>"+bundledHelperSHA256+"</string>")
}

// SetInstalled delegates authorization to macOS. The app never reads a password.
func SetInstalled(ctx context.Context, installed bool) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	app := filepath.Dir(filepath.Dir(filepath.Dir(executable)))
	if filepath.Ext(app) != ".app" {
		return errors.New("请从 Hello TSNet 应用包管理路由助手")
	}
	if out, err := exec.CommandContext(ctx, "/usr/bin/codesign", "--verify", "--deep", "--strict", app).CombinedOutput(); err != nil {
		return fmt.Errorf("应用签名验证失败：%s", strings.TrimSpace(string(out)))
	}
	owner := strconv.Itoa(os.Getuid())
	args := []string{"-", routeHelperScript, "hello-tsnet-route-helper", "uninstall-from-app", owner}
	if installed {
		if bundledHelperSHA256 == "" {
			return errors.New("应用包缺少路由助手校验值")
		}
		args = []string{"-", routeHelperScript, "hello-tsnet-route-helper", "install-from-app", owner, app, bundledHelperSHA256}
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/osascript", args...)
	cmd.Stdin = strings.NewReader(manageScript)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("macOS 未能完成操作：%s", strings.TrimSpace(string(out)))
	}
	switch strings.TrimSpace(string(out)) {
	case "cancelled":
		return nil
	case "completed":
		return confirmHelperOperation(installed, NeedsInstall, NeedsUpgrade)
	default:
		return errors.New("未能确认路由助手操作已完成")
	}
}

func confirmHelperOperation(installed bool, needsInstall, needsUpgrade func() bool) error {
	// Bootstrap has succeeded, but launchd may still be publishing the socket.
	// Confirm the installed version and let route sync wait for the socket.
	if (installed && needsUpgrade()) || (!installed && !needsInstall()) {
		return errors.New("未能确认路由助手的安装状态")
	}
	return nil
}
