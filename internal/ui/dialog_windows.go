//go:build windows

package ui

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/openbiss/openbiss/internal/i18n"
)

// NewNative returns the OS-native DialogProvider for Windows (PowerShell-based).
// Call this once at startup and pass the result via constructor injection.
func NewNative() DialogProvider { return &windowsDialog{} }

// windowsDialog implements DialogProvider using PowerShell on Windows.
// PowerShell is available on all supported Windows versions (7+) and does not
// require additional dependencies.
type windowsDialog struct{}

// PromptPIN shows a Windows InputBox via PowerShell with password masking.
// We use the Microsoft.VisualBasic.Interaction.InputBox COM helper which
// provides a native-looking dialog without requiring any UI framework.
func (d *windowsDialog) PromptPIN(title, message string) ([]byte, error) {
	// The PowerShell script loads the VisualBasic assembly and calls InputBox.
	// InputBox does NOT mask characters by default, so we use a WinForms
	// password prompt for proper masking.
	script := fmt.Sprintf(`
		Add-Type -AssemblyName System.Windows.Forms
		$form = New-Object System.Windows.Forms.Form
		$form.Text = '%s'
		$form.Size = New-Object System.Drawing.Size(350,150)
		$form.StartPosition = 'CenterScreen'
		$form.TopMost = $true
		$label = New-Object System.Windows.Forms.Label
		$label.Location = New-Object System.Drawing.Point(10,10)
		$label.Size = New-Object System.Drawing.Size(320,20)
		$label.Text = '%s'
		$textBox = New-Object System.Windows.Forms.TextBox
		$textBox.Location = New-Object System.Drawing.Point(10,35)
		$textBox.Size = New-Object System.Drawing.Size(310,20)
		$textBox.PasswordChar = '*'
		$okButton = New-Object System.Windows.Forms.Button
		$okButton.Location = New-Object System.Drawing.Point(155,70)
		$okButton.Size = New-Object System.Drawing.Size(75,23)
		$okButton.Text = 'OK'
		$okButton.DialogResult = [System.Windows.Forms.DialogResult]::OK
		$cancelButton = New-Object System.Windows.Forms.Button
		$cancelButton.Location = New-Object System.Drawing.Point(245,70)
		$cancelButton.Size = New-Object System.Drawing.Size(75,23)
		$cancelButton.Text = 'Cancel'
		$cancelButton.DialogResult = [System.Windows.Forms.DialogResult]::Cancel
		$form.Controls.AddRange(@($label, $textBox, $okButton, $cancelButton))
		$form.AcceptButton = $okButton
		$form.CancelButton = $cancelButton
		$result = $form.ShowDialog()
		if ($result -eq [System.Windows.Forms.DialogResult]::OK) {
			Write-Output $textBox.Text
		} else {
			exit 1
		}
	`, escapePS(title), escapePS(message))

	out, err := runPowerShell(script)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, ErrCancelled
		}
		return promptPINFromStdin(message)
	}

	return bytes.TrimRight(out, "\r\n"), nil
}

// SelectCertificate shows a list box dialog via PowerShell WinForms.
func (d *windowsDialog) SelectCertificate(title string, options []string) (int, error) {
	// Build the PowerShell array literal.
	quoted := make([]string, len(options))
	for i, opt := range options {
		quoted[i] = fmt.Sprintf("'%s'", escapePS(opt))
	}
	arrayLiteral := "@(" + strings.Join(quoted, ",") + ")"

	script := fmt.Sprintf(`
		Add-Type -AssemblyName System.Windows.Forms
		$items = %s
		$form = New-Object System.Windows.Forms.Form
		$form.Text = '%s'
		$form.Size = New-Object System.Drawing.Size(500,300)
		$form.StartPosition = 'CenterScreen'
		$form.TopMost = $true
		$listBox = New-Object System.Windows.Forms.ListBox
		$listBox.Location = New-Object System.Drawing.Point(10,10)
		$listBox.Size = New-Object System.Drawing.Size(465,200)
		$listBox.Items.AddRange($items)
		$listBox.SelectedIndex = 0
		$okButton = New-Object System.Windows.Forms.Button
		$okButton.Location = New-Object System.Drawing.Point(310,220)
		$okButton.Size = New-Object System.Drawing.Size(75,23)
		$okButton.Text = 'OK'
		$okButton.DialogResult = [System.Windows.Forms.DialogResult]::OK
		$cancelButton = New-Object System.Windows.Forms.Button
		$cancelButton.Location = New-Object System.Drawing.Point(400,220)
		$cancelButton.Size = New-Object System.Drawing.Size(75,23)
		$cancelButton.Text = 'Cancel'
		$cancelButton.DialogResult = [System.Windows.Forms.DialogResult]::Cancel
		$form.Controls.AddRange(@($listBox, $okButton, $cancelButton))
		$form.AcceptButton = $okButton
		$form.CancelButton = $cancelButton
		$result = $form.ShowDialog()
		if ($result -eq [System.Windows.Forms.DialogResult]::OK) {
			Write-Output $listBox.SelectedIndex
		} else {
			exit 1
		}
	`, arrayLiteral, escapePS(title))

	out, err := runPowerShell(script)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return 0, fmt.Errorf("%s", i18n.T("error.cert_cancelled"))
		}
		return selectFromStdin(options)
	}

	n, err := strconv.Atoi(strings.TrimRight(string(out), "\r\n"))
	if err != nil || n < 0 || n >= len(options) {
		return 0, fmt.Errorf("unexpected selection index from PowerShell")
	}

	return n, nil
}

// runPowerShell executes a PowerShell script string and returns stdout.
func runPowerShell(script string) ([]byte, error) {
	cmd := exec.Command("powershell.exe",
		"-NonInteractive",
		"-NoProfile",
		"-WindowStyle", "Hidden",
		"-Command", script,
	)
	return cmd.Output()
}

// escapePS escapes single quotes for use inside PowerShell single-quoted strings.
func escapePS(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func promptPINFromStdin(message string) ([]byte, error) {
	fmt.Fprintf(os.Stderr, "%s: ", message)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return []byte(scanner.Text()), nil
	}
	return nil, fmt.Errorf("%s", i18n.T("error.read_pin_stdin"))
}

func selectFromStdin(options []string) (int, error) {
	fmt.Fprintln(os.Stderr, i18n.T("dialog.select_cert"))
	for i, opt := range options {
		fmt.Fprintf(os.Stderr, "  [%d] %s\n", i+1, opt)
	}
	fmt.Fprint(os.Stderr, i18n.T("dialog.enter_number"))

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return 0, fmt.Errorf("%s", i18n.T("error.read_selection_stdin"))
	}

	n, err := strconv.Atoi(strings.TrimSpace(scanner.Text()))
	if err != nil || n < 1 || n > len(options) {
		return 0, fmt.Errorf("%s", i18n.T("error.invalid_selection"))
	}

	return n - 1, nil
}
