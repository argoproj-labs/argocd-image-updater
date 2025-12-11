package os

import (
	"fmt"
	"os/exec"

	//lint:ignore ST1001 "This is a common practice in Gomega tests for readability."
	. "github.com/onsi/ginkgo/v2" //nolint:all
)

func ExecCommand(cmdArgs ...string) (string, error) {
	return ExecCommandWithOutputParam(true, true, cmdArgs...)
}

// ExecCommandWithOutputParam You probably want to use ExecCommand, unless you need to suppress the output of sensitive data (for example, openssl CLI output)
func ExecCommandWithOutputParam(printOutput bool, printCommand bool, cmdArgs ...string) (string, error) {
	if len(cmdArgs) == 0 {
		return "", fmt.Errorf("no command arguments provided")
	}

	if printCommand {
		GinkgoWriter.Println("executing command:", cmdArgs)
	}

	// #nosec G204
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)

	outputBytes, err := cmd.CombinedOutput()

	var output string
	if outputBytes != nil {
		output = string(outputBytes)
	}

	if printOutput {
		GinkgoWriter.Println(output)
	}

	return output, err
}
