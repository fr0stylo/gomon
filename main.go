package main

import (
	"context"
	"flag"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

func pipeToStream(src *io.ReadCloser, dest io.WriteCloser) {
	for {
		io.Copy(dest, *src)
	}
}

var wg sync.WaitGroup

func runProcess(ctx context.Context, command []string, dir string) {
	c := exec.CommandContext(ctx, command[0], command[1:]...)
	c.Dir = dir
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
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

	if err = c.Start(); err != nil {
		log.Fatal(err)
	}

	if err = c.Wait(); err != nil {
		syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
		wg.Done()
	}
}

func runner(restartCh chan struct{}, dir string, commandToExecute []string) {
	ctx, cancel := context.WithCancel(context.Background())
	wg.Add(1)
	go runProcess(ctx, commandToExecute, dir)
	for range restartCh {
		log.Print("Killing process")
		cancel()

		wg.Wait()
		log.Print("Starting new process")
		ctx, cancel = context.WithCancel(context.Background())
		wg.Add(1)
		go runProcess(ctx, commandToExecute, dir)
	}

	defer cancel()
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
