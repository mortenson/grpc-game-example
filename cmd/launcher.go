package main

import (
	"fmt"
	"os/exec"
	"runtime"
)

var Command string

func main() {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/C", "start", ".\\tshooter_windows_"+Command)
	case "linux":
		cmd = exec.Command("x-terminal-emulator", "-e", "./tshooter_linux_"+Command)
	case "darwin":
		cmd = exec.Command("open", "Terminal", "./tshooter_darwin_"+Command)
	default:
		panic("unknown runtime")
	}
	err := cmd.Start()
	if err != nil {
		panic(fmt.Sprintf("%+v", err))
	}
}
