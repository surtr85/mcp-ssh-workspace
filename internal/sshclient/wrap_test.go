package sshclient

import (
	"os/exec"
	"testing"
)

func TestShellWrappers(t *testing.T) {
	testCmds := []string{
		`echo "hello world"`,
		`echo 'single quotes' and "double quotes"`,
		`python3 -c 'print("hello (parentheses) and $vars")'`,
		`ls -la | grep "something"`,
	}

	shells := []string{"bash", "sh"}
	if p, err := exec.LookPath("fish"); err == nil {
		shells = append(shells, p)
	}

	for _, sh := range shells {
		for _, cmd := range testCmds {
			fullCmd, _, _ := WrapCommand(cmd, "/")

			out, err := exec.Command(sh, "-c", fullCmd).CombinedOutput()
			if err != nil {
				t.Errorf("[%s] failed for cmd %q:\nErr: %v\nOutput: %s", sh, cmd, err, string(out))
			} else {
				t.Logf("[%s] succeeded for %q:\n%s", sh, cmd, string(out))
			}
		}
	}
}
