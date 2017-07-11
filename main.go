package main

import (
	"fmt"
	"net"
	"os"
	"syscall"
)

func main() {
	err := run()
	if err != nil {
		fmt.Printf("err: %v\n", err)
		os.Exit(1)
	}

	os.Exit(0)
}

func run() error {
	srcPath := os.Args[1]
	dstPath := os.Args[2]
	fmt.Printf("Relaying %s to %s\n", srcPath, dstPath)

	dstSocket, err := net.Listen("unix", dstPath)
	if err != nil {
		return err
	}

	for {
		fmt.Printf("Ready for new clients\n")

		dstConn, err := dstSocket.Accept()
		if err != nil {
			continue
		}
		fmt.Printf("A new client connected\n")

		srcConn, err := net.Dial("unix", srcPath)
		if err != nil {
			dstConn.Close()
			continue
		}

		fmt.Printf("Connection established to target\n")

		relay := func(src *net.UnixConn, dst *net.UnixConn) {
			for {
				dataBuf := make([]byte, 4096)
				oobBuf := make([]byte, 4096)

				// Read from the source
				sData, sOob, _, _, err := src.ReadMsgUnix(dataBuf, oobBuf)
				if err != nil {
					fmt.Printf("Disconnected during read: %v\n", err)
					src.Close()
					dst.Close()
					return
				}

				var fds []int
				if sOob > 0 {
					entries, err := syscall.ParseSocketControlMessage(oobBuf[:sOob])
					if err != nil {
						fmt.Printf("Failed to parse control message: %v\n", err)
						src.Close()
						dst.Close()
						return
					}

					for _, msg := range entries {
						fds, err = syscall.ParseUnixRights(&msg)
						if err != nil {
							fmt.Printf("Failed to get fds list for control message: %v\n", err)
							src.Close()
							dst.Close()
							return
						}
					}
				}

				// Send to the destination
				tData, tOob, err := dst.WriteMsgUnix(dataBuf[:sData], oobBuf[:sOob], nil)
				if err != nil {
					fmt.Printf("Disconnected during write: %v\n", err)
					src.Close()
					dst.Close()
					return
				}

				if sData != tData || sOob != tOob {
					fmt.Printf("Some data got lost during transfer, disconnecting.")
					src.Close()
					dst.Close()
					return
				}

				// Close those fds we received
				if fds != nil {
					for _, fd := range fds {
						err := syscall.Close(fd)
						if err != nil {
							fmt.Printf("Failed to close fd %d: %v\n", fd, err)
							src.Close()
							dst.Close()
							return
						}
					}
				}
			}
		}

		go relay(srcConn.(*net.UnixConn), dstConn.(*net.UnixConn))
		go relay(dstConn.(*net.UnixConn), srcConn.(*net.UnixConn))
		fmt.Printf("Relay started\n")
	}

	return nil
}
