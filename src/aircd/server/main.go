package main

import (
    "net"
    "sync"
    "log"

    _ "net/http/pprof"
    "net/http"
)

var logger *log.Logger

func main() {
    go func() {
        log.Println(http.ListenAndServe("localhost:6060", nil))
    }()

    log.Print("Starting server")

    listener, _ := net.Listen("tcp", ":6667")

    server := Server{"example.example.com", []*Channel{}, []*User{}, sync.Mutex{}}

    for {
        conn, err := listener.Accept()
        if err != nil {
            log.Printf("Error: %v", err)
            continue
        }

        user := NewUser(&server, conn)
        go user.serve()
    }
}