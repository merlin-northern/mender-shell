// Copyright 2020 Northern.tech AS
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.
package shell

import (
	"bufio"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/mendersoftware/mender-shell/connection"
	log "github.com/sirupsen/logrus"

	ws2 "github.com/mendersoftware/go-lib-micro/ws"
	wsshell "github.com/mendersoftware/go-lib-micro/ws/shell"
)

var (
	ErrExecWriteBytesShort = errors.New("failed to write the whole message")
)

// MenderShellMessage represents a message between the device and the backend
type MenderShellMessage struct {
	//type of message, used to determine the meaning of data
	Type string `json:"type" msgpack:"type"`
	//session id, as returned to the caller in a response to the MessageTypeSpawnShell
	//message.
	SessionId string `json:"session_id" msgpack:"session_id"`
	//message status, currently normal and error message types are supported
	Status wsshell.MenderShellMessageStatus `json:"status_code" msgpack:"status_code"`
	//the message payload, if
	// * .Type===MessageTypeShellCommand interpreted as keystrokes and passed
	//   to the stdin of the terminal running the shell.
	// * .Type===MessageTypeSpawnShell interpreted as user_id and passed
	//   to the session.NewMenderShellSession.
	Data []byte `json:"data" msgpack:"data"`
}

type MenderShell struct {
	writeMutex *sync.Mutex
	sessionId  string
	ws         *connection.Connection
	r          io.Reader
	w          io.Writer
	running    bool
}

type MenderShellCommand struct {
	path string
}

//Create a new shell, note that we assume that r Reader and w Writer
//are already connected to the i/o of the shell process and ws websocket
//is connected and ws.SetReadDeadline(time.Now().Add(defaultPingWait))
//was already called and ping-pong was established
func NewMenderShell(sessionId string, writeMutex *sync.Mutex, ws *connection.Connection, r io.Reader, w io.Writer) *MenderShell {
	shell := MenderShell{
		writeMutex: writeMutex,
		sessionId:  sessionId,
		ws:         ws,
		r:          r,
		w:          w,
		running:    false,
	}
	return &shell
}

func (s *MenderShell) GetWriteTimeout() time.Duration {
	return s.ws.GetWriteTimeout()
}

func (s *MenderShell) Start() {
	go s.pipeStdout()
	s.running = true
}

func (s *MenderShell) Stop() {
	s.running = false
}

func (s *MenderShell) IsRunning() bool {
	return s.running
}

func (s *MenderShell) UpdateWSConnection(ws *connection.Connection) error {
	s.ws = ws
	return nil
}

func (s *MenderShell) pipeStdout() {
	sr := bufio.NewReader(s.r)
	for {
		if !s.IsRunning() {
			return
		}
		raw := make([]byte, 255)
		n, err := sr.Read(raw)
		if err != nil {
			if !s.IsRunning() {
				return
			}

			log.Errorf("error reading stdout: '%s'; restart is needed.", err)
			break
		}
		//m := &MenderShellMessage{
		//	Type:      MessageTypeShellCommand,
		//	Data:      raw[:n],
		//	SessionId: s.sessionId,
		//}
		//data, err := msgpack.Marshal(m)
		//if err != nil {
		//	log.Errorf("error parsing message: %s", err)
		//	continue
		//}
		//s.ws.WriteMessageRaw(data)

		msg := &ws2.ProtoMsg{
			Header: ws2.ProtoHdr{
				Proto:     ws2.ProtoTypeShell,
				MsgType:   wsshell.MessageTypeShellCommand,
				SessionID: s.sessionId,
				Properties: map[string]interface{}{
					"status": wsshell.NormalMessage,
				},
			},
			Body: raw[:n],
		}
		err = s.ws.WriteMessage(msg)
	}
}
