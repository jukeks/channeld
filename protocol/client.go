package protocol

import (
	"bufio"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

type IrcConnection struct {
	conn   net.Conn
	reader *bufio.Reader

	mutex  sync.Mutex
	closed bool

	messageCounter uint32
	counterReseted time.Time

	incoming chan ClientAction
	outgoing chan string
	quit     chan bool
}

func NewIrcConnection(conn net.Conn,
	incoming chan ClientAction) *IrcConnection {
	c := new(IrcConnection)

	c.conn = conn
	c.reader = bufio.NewReader(conn)
	c.mutex = sync.Mutex{}

	c.counterReseted = time.Now()

	c.incoming = incoming
	c.outgoing = make(chan string, 1000)
	c.quit = make(chan bool, 2)

	return c
}

func (conn *IrcConnection) Close() {
	conn.mutex.Lock()
	defer conn.mutex.Unlock()

	if !conn.closed {
		conn.closed = true

		conn.conn.Close()
		conn.quit <- true
		conn.quit <- true
	}
}

func (conn *IrcConnection) Send(msg string) {
	select {
	case conn.outgoing <- msg:
		return
	default:
		log.Printf("Client %v queue is full. Closing.", conn)
		conn.incoming <- ClientAction{conn, nil}
	}
}

func (conn *IrcConnection) SendMessage(message IrcMessage) {
	conn.Send(message.Serialize())
}

func (conn *IrcConnection) SendMessageFrom(from string, message IrcMessage) {
	conn.Send(GetSerializedMessageFrom(from, message))
}

/*----------------------------------------------------------------------------*/

func (conn *IrcConnection) checkCounter() bool {
	if time.Now().After(conn.counterReseted.Add(10 * time.Second)) {
		conn.messageCounter = 0
		conn.counterReseted = time.Now()
	}

	conn.messageCounter += 1
	if conn.messageCounter > 10 {
		return false
	}

	return true
}

func (conn *IrcConnection) getHostname() string {
	split := strings.Split(conn.conn.RemoteAddr().String(), ":")
	remote := split[0]

	names, _ := net.LookupAddr(remote)
	if len(names) > 0 {
		remote = names[0]
	}

	return remote
}

func (conn *IrcConnection) Serve(newClients chan ConnectionInitiationAction) {
	succ := conn.handshake(newClients)
	if !succ {
		log.Printf("Handshake failed")
		return
	}

	go conn.writerRoutine()

	for {
		select {
		case <-conn.quit:
			return
		default:
		}

		message, err := conn.readMessage()
		if err != nil {
			log.Printf("read failed: %v", err)
			conn.incoming <- ClientAction{conn, nil}
			return
		}

		if !conn.checkCounter() {
			log.Printf("Client flooding %d messages in %s",
				conn.messageCounter,
				time.Since(conn.counterReseted).String())
			conn.incoming <- ClientAction{conn, nil}
			return
		}

		conn.incoming <- ClientAction{conn, message}
	}
}

func (conn *IrcConnection) writerRoutine() {
	for {
		select {
		case msg := <-conn.outgoing:
			conn.write(msg)
		case <-conn.quit:
			return
		}
	}
}

func (conn *IrcConnection) write(message string) {
	err := WriteLine(conn.conn, message)
	if err != nil {
		log.Printf("Error writing socket %v", err)
		conn.incoming <- ClientAction{conn, nil}
		return
	}

	log.Printf("Wrote: %s", message)
}

func (conn *IrcConnection) readMessage() (IrcMessage, error) {
	line, err := ReadLine(conn.reader)
	if err != nil {
		return nil, err
	}

	return ParseMessage(line), nil
}
