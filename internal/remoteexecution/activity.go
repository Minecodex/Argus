package remoteexecution

import "strings"

// Only fixed labels may cross the chat boundary. Arbitrary identifiers, script
// fragments and arguments can contain secrets and never become activity text.
func activityCommandLabel(script string) []string {
	labels := []string{}
	seen := map[string]bool{}
	known := map[string]bool{}
	for _, name := range strings.Fields("kubectl systemctl journalctl ps top free df du ss ip cat tail head grep find ls pwd echo printf sleep curl wget bash sh python python3 Get-Process Get-Service Restart-Service Stop-Service Start-Service Get-Content Get-ChildItem Get-EventLog Get-WinEvent Get-Counter Get-NetTCPConnection Test-NetConnection Set-Location Write-Output Start-Sleep") {
		known[name] = true
	}
	for _, line := range strings.Split(script, "\n") {
		words := strings.Fields(line)
		if len(words) == 0 {
			continue
		}
		label := "shell command (details withheld)"
		if known[words[0]] {
			label = words[0] + " (arguments withheld)"
		}
		if !seen[label] {
			seen[label] = true
			labels = append(labels, label)
		}
		if len(labels) == 4 {
			break
		}
	}
	return labels
}
