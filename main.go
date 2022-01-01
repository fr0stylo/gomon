package main

import (
	"flag"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func pipeToStream(src *io.ReadCloser, dest io.WriteCloser) {
	for {
		io.Copy(dest, *src)
	}
}

func runProcess(command []string, dir string) *exec.Cmd {
	c := exec.Command(command[0], command[1:]...)
	c.Dir = dir
	stdout, err := c.StdoutPipe()
	if err != nil {
		log.Fatal(err)
	}

	stderr, err := c.StderrPipe()
	if err != nil {
		log.Fatal(err)
	}
	go pipeToStream(&stderr, os.Stderr)
	go pipeToStream(&stdout, os.Stdout)

	err = c.Start()
	if err != nil {
		log.Fatal(err)
	}

	return c
}

func runner(restartCh chan struct{}, dir string, commandToExecute []string) {
	c := runProcess(commandToExecute, dir)
	for range restartCh {
		c.Process.Signal(syscall.SIGKILL)
		c.Wait()
		c = runProcess(commandToExecute, dir)
	}
}

func main() {
	ext := flag.String("ext", "", "File extensions to track")
	dir := flag.String("dir", ".", "Directory to watch, command will be run in this conetext")
	t := flag.Duration("t", time.Second*5, "Directory to watch")
	flag.Parse()

	commandToExecute := flag.Args()

	channel := make(chan struct{})

	go runner(channel, *dir, commandToExecute)

	for {
		changed := false
		filepath.Walk(*dir, func(path string, info fs.FileInfo, err error) error {
			if info.IsDir() {
				return nil
			}

			timeElapsed := time.Since(info.ModTime())
			if strings.HasSuffix(path, *ext) && timeElapsed < (time.Second*6) {
				changed = true
			}

			return nil
		})
		if changed {
			channel <- struct{}{}
		}

		time.Sleep(*t)
	}
}
