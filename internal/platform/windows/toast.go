package windows

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// SendToast sends a native Windows notification if running on Windows
func SendToast(title, message string) error {
	if runtime.GOOS != "windows" {
		// Non-windows environment, no-op / log
		return nil
	}

	// Escape strings for powershell
	safeTitle := strings.ReplaceAll(title, "'", "''")
	safeMessage := strings.ReplaceAll(message, "'", "''")

	psCmd := fmt.Sprintf(`
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] > $null
$template = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$textNodes = $template.GetElementsByTagName("text")
$textNodes.Item(0).AppendChild($template.CreateTextNode('%s')) > $null
$textNodes.Item(1).AppendChild($template.CreateTextNode('%s')) > $null
$toast = [Windows.UI.Notifications.ToastNotification]::new($template)
$notifier = [Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('StudyGuardian')
$notifier.Show($toast)
`, safeTitle, safeMessage)

	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", psCmd)
	return cmd.Run()
}
