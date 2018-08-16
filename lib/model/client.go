package model

import (
	"bytes"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/WangYihang/Platypus/lib/util/hash"
	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/WangYihang/Platypus/lib/util/str"
	humanize "github.com/dustin/go-humanize"
)

type Client struct {
	TimeStamp   time.Time
	Conn        net.Conn
	Interactive bool
	Group       bool
	Hash        string
	ReadLock    *sync.Mutex
	WriteLock   *sync.Mutex
}

func CreateClient(conn net.Conn) *Client {
	client := &Client{
		TimeStamp:   time.Now(),
		Conn:        conn,
		Interactive: false,
		Group:       false,
		Hash:        hash.MD5(conn.RemoteAddr().String()),
		ReadLock:    new(sync.Mutex),
		WriteLock:   new(sync.Mutex),
	}
	return client
}

func (c *Client) Close() {
	log.Info("Closeing client: %s", c.Desc())
	c.Conn.Close()
}

func (c *Client) Desc() string {
	addr := c.Conn.RemoteAddr()
	return fmt.Sprintf("[%s] %s://%s (connected at: %s) [%t]", c.Hash, addr.Network(), addr.String(), humanize.Time(c.TimeStamp), c.Interactive)
}

func (c *Client) Readfile(filename string) string {
	if c.FileExists(filename) {
		return c.SystemToken("cat " + filename)
	} else {
		return ""
	}
}

func (c *Client) FileExists(path string) bool {
	return c.SystemToken("ls "+path) == path+"\n"
}

func (c *Client) System(command string) {
	c.Conn.Write([]byte(command + "\n"))
}

func (c *Client) SystemToken(command string) string {
	tokenA := str.RandomString(0x10)
	tokenB := str.RandomString(0x10)
	input := "echo " + tokenA + " && " + command + "; echo " + tokenB
	c.System(input)
	c.ReadUntil(tokenA)
	output := c.ReadUntil(tokenB)
	log.Info(output)
	return output
}

func (c *Client) ReadUntil(token string) string {
	inputBuffer := make([]byte, 1)
	var outputBuffer bytes.Buffer
	for {
		c.ReadLock.Lock()
		n, err := c.Conn.Read(inputBuffer)
		c.ReadLock.Unlock()
		if err != nil {
			log.Error("Read from client failed")
			c.Interactive = false
			Ctx.DeleteClient(c)
			return outputBuffer.String()
		}
		outputBuffer.Write(inputBuffer[:n])
		// If found token, then finish reading
		if strings.HasSuffix(outputBuffer.String(), token) {
			break
		}
	}
	log.Info("%d bytes read from client", len(outputBuffer.String()))
	return outputBuffer.String()
}

func (c *Client) Read(timeout time.Duration) (string, bool) {
	// Set read time out
	c.Conn.SetReadDeadline(time.Now().Add(timeout))

	inputBuffer := make([]byte, 1024)
	var outputBuffer bytes.Buffer
	var is_timeout bool
	for {
		c.ReadLock.Lock()
		n, err := c.Conn.Read(inputBuffer)
		c.ReadLock.Unlock()
		if err != nil {

			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// log.Info("Read timeout")
				is_timeout = true
			} else {
				log.Error("Read from client failed")
				c.Interactive = false
				Ctx.DeleteClient(c)
				is_timeout = false
			}
			break
		}
		// log.Info("%d bytes read from client", n)
		// If read size equals zero, then finish reading
		if n == 0 {
			break
		}
		outputBuffer.Write(inputBuffer[:n])
	}
	// log.Info("%d bytes read from client totally", len(outputBuffer.String()))

	// Reset read time out
	c.Conn.SetReadDeadline(time.Time{})

	return outputBuffer.String(), is_timeout
}

func (c *Client) Write(data []byte) int {
	c.WriteLock.Lock()
	n, err := c.Conn.Write(data)
	c.WriteLock.Unlock()
	if err != nil {
		log.Error("Write to client failed")
		c.Interactive = false
		Ctx.DeleteClient(c)
	}
	log.Info("%d bytes sent to client", n)
	return n
}
