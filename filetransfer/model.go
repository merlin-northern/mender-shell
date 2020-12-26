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
/*
The typical flow for a file transfer:

mender-shell   +                 +   the other side (UI,cli,...)
               |                 |
               +<----stat_file---+ sent to get the size of the file, and its existential data
               |(path)           |
               +---size,modes--->+ got the size, and the mode, now can decide on the chunk size
               |                 |
               +<----get_file----+ to get the file
               |(path,chunk_size)|
               |                 |
               +---file_chunk--->+ each chunk carries number, size and data, we can calculate %%
               |number,size,data |
               |.                |
               |.                |
               |.                |
               +---file_chunk--->+ last chunk received
               |number,size,data |
               |                 |
               +                 +
*/
package filetransfer

import (
	"github.com/mendersoftware/go-lib-micro/ws"
	"github.com/mendersoftware/go-lib-micro/ws/filetransfer"
	"github.com/mendersoftware/mender-shell/connectionmanager"
	log "github.com/sirupsen/logrus"
	//"io"
	"os"
)

type Message struct {
	//type of message, used to determine the meaning of data
	Type string `json:"type" msgpack:"type"`
	//session id, as returned to the caller in a response to the MessageTypeSpawnShell
	//message.
	SessionId string `json:"session_id" msgpack:"session_id"`
	//user id contains the ID of the user
	UserId string `json:"user_id" msgpack:"user_id"`
	//message status, currently normal and error message types are supported
	Status filetransfer.MenderFileTransferMessageStatus `json:"status_code" msgpack:"status_code"`
	//path to the file
	FilePath string `json:"file_path" msgpack:"file_path"`
	//chunk number, starting from 1, caller knows from the MessageTypeStatFile reply how many chunks to expect
	ChunkNumber int64 `json:"chunk_number" msgpack:"chunk_number"`
	//chunk size in bytes
	ChunkSize int64 `json:"chunk_size" msgpack:"chunk_size"`
	//chunk data, take ChunkSize from there
	ChunkData []byte `json:"chunk_data" msgpack:"chunk_data"`
}

func GetFileSize(path string) int64 {
	f, err := os.Stat("path")
	if err != nil {
		return -1
	}
	return f.Size()
}

func SendStat(path string, sessionId string) error {
	var size int64
	var messageType filetransfer.MenderFileTransferMessageStatus

	f, err := os.Stat(path)
	if err != nil {
		size=-1
		messageType=filetransfer.ErrorMessage
		return err
	} else {
    size=f.Size()
		messageType=filetransfer.NormalMessage
	}

	msg := &ws.ProtoMsg{
		Header: ws.ProtoHdr{
			Proto:     ws.ProtoTypeFileTransfer,
			MsgType:   filetransfer.MessageTypeStatFile,
			SessionID: sessionId,
			Properties: map[string]interface{}{
				"file_size":   size,
				"status": messageType,
			},
		},
		Body: nil,
	}
	err = connectionmanager.Write(ws.ProtoTypeFileTransfer, msg)
	if err != nil {
		log.Debugf("error on write: %s", err.Error())
		return err
	}
	return nil
}

//send the file, meant to be run from a separate go routine
func SendFile(path string, chunkSize int64, sessionId string) {
	f, err := os.Open(path)
	if err != nil {
		//send error message here
		return
	}

	defer f.Close()
	_, err = f.Stat()
	if err != nil {
		//send error message here
		return
	}

	//size := stat.Size()
	//remainingSize := size
	chunkNumber := 0

	totalBytesSent:=0
	buffer := make([]byte, chunkSize)
	for {
		//var bufferLength int64
		//if remainingSize <= chunkSize {
		//	bufferLength = remainingSize
		//} else {
		//	bufferLength = chunkSize
		//}
		log.Debug("reading from file")
		//n, err := io.ReadAtLeast(f, buffer, int(bufferLength))
		n,err := f.Read(buffer)
		log.Debugf("read: %d err=%v", n, err)
		if err != nil {
			//send error message here
			log.Debugf("returning on err=%v totalBytesSent=%d", err, totalBytesSent)
			return
		}

		log.Debug("sending file chunk.")
		msg := &ws.ProtoMsg{
			Header: ws.ProtoHdr{
				Proto:     ws.ProtoTypeFileTransfer,
				MsgType:   filetransfer.MessageTypeFileChunk,
				SessionID: sessionId,
				Properties: map[string]interface{}{
					"chunk_size":   n,
					"chunk_number": chunkNumber,
				},
			},
			Body: buffer[:n],
		}
		totalBytesSent+=n
		err = connectionmanager.Write(ws.ProtoTypeFileTransfer, msg)
		if err != nil {
			log.Debugf("error on write: %s", err.Error())
			return
		}
		log.Debugf("sent file chunk len=%d",len(msg.Body))

		chunkNumber++
	}
}
