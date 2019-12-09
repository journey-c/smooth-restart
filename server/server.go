package main

import (
	"flag"
	"fmt"
	"golang.org/x/sys/unix"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

var (
	workSpace string

	logger *log.Logger

	writeTimeout = time.Second * 5
	readTimeout  = time.Second * 5

	signalChan = make(chan os.Signal)

	connFiles sync.Map

	serverListener net.Listener

	isUpdate = false
)

func init() {
	flag.StringVar(&workSpace, "w", ".", "Usage:\n ./server -w=workspace")
	flag.Parse()

	file, err := os.OpenFile(filepath.Join(workSpace, "server.log"), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0777)
	if err != nil {
		panic(err)
	}
	logger = log.New(file, "", 11)
	go beforeStart()
	go signalHandler()
}

func main() {
	var err error
	serverListener, err = net.Listen("tcp", ":7000")
	if err != nil {
		panic(err)
	}
	for {
		if isUpdate == true {
			continue
		}
		conn, err := serverListener.Accept()
		if err != nil {
			logger.Println("conn error")
			continue
		}
		c := conn.(*net.TCPConn)
		go connectionHandler(c)
	}
}

func connectionHandler(conn *net.TCPConn) {
	file, _ := conn.File()
	connFiles.Store(file, true)
	logger.Printf("conn fd %d\n", file.Fd())
	defer func() {
		connFiles.Delete(file)
		_ = conn.Close()
	}()
	for {
		if isUpdate == true {
			continue
		}
		err := conn.SetReadDeadline(time.Now().Add(readTimeout))
		if err != nil {
			logger.Println(err.Error())
			return
		}
		rBuf := make([]byte, 4)
		_, err = conn.Read(rBuf)
		if err != nil {
			logger.Println(err.Error())
			return
		}
		if string(rBuf) != "ping" {
			logger.Println("failed to parse the message " + string(rBuf))
			return
		}
		err = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
		if err != nil {
			logger.Println(err.Error())
			return
		}
		_, err = conn.Write([]byte(`pong`))
		if err != nil {
			logger.Println(err.Error())
			return
		}
	}
}

func beforeStart() {
	connInterface, err := net.Dial("unix", filepath.Join(workSpace, "conn.sock"))
	if err != nil {
		logger.Println(err.Error())
		return
	}
	defer func() {
		_ = connInterface.Close()
	}()

	unixConn := connInterface.(*net.UnixConn)

	b := make([]byte, 1)
	oob := make([]byte, 32)
	for {
		err = unixConn.SetWriteDeadline(time.Now().Add(time.Minute * 3))
		if err != nil {
			fmt.Println(err.Error())
			return
		}
		n, oobn, _, _, err := unixConn.ReadMsgUnix(b, oob)
		if err != nil {
			logger.Println(err.Error())
			return
		}
		if n != 1 || b[0] != 0 {
			if n != 1 {
				logger.Printf("recv fd type error: %d\n", n)
			} else {
				logger.Println("init finish")
			}
			return
		}
		scms, err := unix.ParseSocketControlMessage(oob[0:oobn])
		if err != nil {
			logger.Println(err.Error())
			return
		}
		if len(scms) != 1 {
			logger.Printf("recv fd num != 1 : %d\n", len(scms))
			return
		}
		fds, err := unix.ParseUnixRights(&scms[0])
		if err != nil {
			logger.Println(err.Error())
			return
		}
		if len(fds) != 1 {
			logger.Printf("recv fd num != 1 : %d\n", len(fds))
			return
		}
		logger.Printf("recv fd %d\n", fds[0])
		file := os.NewFile(uintptr(fds[0]), "fd-from-old")
		conn, err := net.FileConn(file)
		if err != nil {
			logger.Println(err.Error())
			return
		}
		go connectionHandler(conn.(*net.TCPConn))
	}
}

func signalHandler() {
	signal.Notify(
		signalChan,
		syscall.SIGUSR2,
	)
	for {
		sc := <-signalChan
		switch sc {
		case syscall.SIGUSR2:
			gracefulExit()
		default:
			continue
		}
	}
}

func gracefulExit() {
	var connWait sync.WaitGroup
	_ = syscall.Unlink(filepath.Join(workSpace, "conn.sock"))
	listenerInterface, err := net.Listen("unix", filepath.Join(workSpace, "conn.sock"))
	if err != nil {
		logger.Println(err.Error())
		return
	}
	defer func() {
		_ = listenerInterface.Close()
	}()
	unixListener := listenerInterface.(*net.UnixListener)
	connWait.Add(1)
	go func() {
		defer connWait.Done()
		unixConn, err := unixListener.AcceptUnix()
		if err != nil {
			logger.Println(err.Error())
			return
		}
		defer func() {
			_ = unixConn.Close()
		}()
		connFiles.Range(func(key, value interface{}) bool {
			if key == nil || value == nil {
				return false
			}
			file := key.(*os.File)
			defer func() {
				_ = file.Close()
			}()
			buf := make([]byte, 1)
			buf[0] = 0
			rights := syscall.UnixRights(int(file.Fd()))
			_, _, err := unixConn.WriteMsgUnix(buf, rights, nil)
			if err != nil {
				logger.Println(err.Error())
			}
			logger.Printf("send fd %d\n", file.Fd())
			return true
		})
		finish := make([]byte, 1)
		finish[0] = 1
		_, _, err = unixConn.WriteMsgUnix(finish, nil, nil)
		if err != nil {
			logger.Println(err.Error())
		}
	}()

	isUpdate = true
	execSpec := &syscall.ProcAttr{
		Env:   os.Environ(),
		Files: append([]uintptr{os.Stdin.Fd(), os.Stdout.Fd(), os.Stderr.Fd()}),
	}

	pid, err := syscall.ForkExec(os.Args[0], os.Args, execSpec)
	if err != nil {
		logger.Println(err.Error())
		return
	}
	logger.Printf("old process %d new process %d\n", os.Getpid(), pid)
	_ = serverListener.Close()

	connWait.Wait()
	os.Exit(0)
}
