package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
)

type Message struct {
	Name    string
	Message string
}

type Name struct {
	Name string
}

type Send struct {
	Message string
}

type Writer struct {
	writer io.Writer
}

func NewWriter(writer io.Writer) *Writer {
	return &Writer{writer: writer}
}

func (w *Writer) WriteString(data string) error {
	_, err := w.writer.Write([]byte(data))
	return err
}

func (w *Writer) Write(data interface{}) error {
	switch command := data.(type) {
	case Message:
		_, err := fmt.Fprintf(w.writer, "M %v %v\n", command.Name, command.Message)
		return err
	case Name:
		_, err := fmt.Fprintf(w.writer, "N %v\n", command.Name)
		return err
	case Send:
		_, err := fmt.Fprintf(w.writer, "S %v\n", command.Message)
		return err
	default:
		return fmt.Errorf("Unknown command")
	}
}

type Reader struct {
	reader *bufio.Reader
}

func NewReader(reader io.Reader) *Reader {
	return &Reader{reader: bufio.NewReader(reader)}
}

func (r *Reader) Read() (interface{}, error) {
	message, err := r.reader.ReadString(' ')
	if err != nil {
		return nil, err
	}
	switch message {
	case "M":
		name, err := r.reader.ReadString(' ')
		if err != nil {
			return nil, err
		}
		message, err := r.reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		return Message{Name: name[:len(name)-1], Message: message[:len(message)-1]}, nil
	case "N":
		name, err := r.reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		return Name{Name: name[:len(name)-1]}, nil
	case "S":
		message, err := r.reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		return Send{Message: message[:len(message)-1]}, nil
	default:
		return nil, fmt.Errorf("Unknown command")
	}
}

func (r *Reader) ReadAll() ([]interface{}, error) {
	var commands []interface{}
	for {
		command, err := r.Read()
		if err != nil {
			return nil, err
		}
		if command == nil {
			break
		}
		commands = append(commands, command)
	}
	return commands, nil
}

type Server interface {
	Listen(address string) error
	Close()
	Bind()
	Broadcast(data interface{}) error
}

type TcpServer struct {
	listener net.Listener
	mutex    *sync.Mutex
	clients  []*internalClient
}

type internalClient struct {
	conn   net.Conn
	writer *Writer
	name   string
}

func (s *TcpServer) Listen(address string) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	log.Printf("Listening on %v", address)
	s.listener = listener
	return nil
}

func (s *TcpServer) Close() {
	log.Println("Closing server")
	s.listener.Close()
}

func (s *TcpServer) Accept(conn net.Conn) *internalClient {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	client := &internalClient{conn: conn, writer: NewWriter(conn)}
	s.clients = append(s.clients, client)
	log.Printf("Accepted connection from %v", conn.RemoteAddr())
	return client
}

func (s *TcpServer) Remove(client *internalClient) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	for i, c := range s.clients {
		if c == client {
			s.clients = append(s.clients[:i], s.clients[i+1:]...)
			break
		}
	}
	log.Printf("Closing connection from %v", client.conn.RemoteAddr())
	client.conn.Close()
}

func (s *TcpServer) Bind() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			log.Println(err)
			continue
		}
		client := s.Accept(conn)
		go s.handle(client)
	}
}

func (s *TcpServer) handle(client *internalClient) {
	reader := NewReader(client.conn)
	for {
		command, err := reader.Read()
		if err != nil && err != io.EOF {
			log.Println(err)
			break
		}
		if command == nil {
			switch innerCommand := command.(type) {
			case Send:
				log.Printf("Received MESSAGE from %v: %v", client.name, innerCommand.Message)
				go s.Broadcast(Message{Name: client.name, Message: innerCommand.Message})
			case Name:
				log.Printf("Received NAME from %v: %v", client.conn.RemoteAddr(), innerCommand.Name)
				client.name = innerCommand.Name
			}
		}
		if err == io.EOF {
			break
		}
	}
}

func (s *TcpServer) Broadcast(data interface{}) error {
	for _, client := range s.clients {
		client.writer.Write(data)
	}
	return nil
}

func NewTcpServer() *TcpServer {
	return &TcpServer{mutex: &sync.Mutex{}}
}

func main() {
	var server Server = NewTcpServer()
	err := server.Listen(":8080")
	if err != nil {
		log.Fatal(err)
	}
	server.Bind()
}
