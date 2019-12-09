package main

import (
	"fmt"
	"net"
	"time"
)

var (

	writeTimeout = time.Second * 5
	readTimeout  = time.Second * 5
)

func main() {
	conn, err := net.Dial("tcp", "127.0.0.1:7000")
	if err != nil {
		panic(err)
	}
	defer func() {
		conn.Close()
	}()
	for {
		time.Sleep(time.Second)
		err := conn.SetWriteDeadline(time.Now().Add(writeTimeout))
		if err != nil {
			fmt.Println(err.Error())
			break
		}
		fmt.Println("send ping")
		_, err = conn.Write([]byte(`ping`))
		if err != nil {
			fmt.Println(err.Error())
			break
		}
		err = conn.SetReadDeadline(time.Now().Add(readTimeout))
		if err != nil {
			fmt.Println(err.Error())
			break
		}
		rBuf := make([]byte, 4)
		_, err = conn.Read(rBuf)
		if err != nil {
			fmt.Println(err.Error())
		}
		fmt.Println("recv " + string(rBuf))
	}
}
