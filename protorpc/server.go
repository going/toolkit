package protorpc

import (
	"bufio"
	"code.google.com/p/goprotobuf/proto"
	"errors"
	"fmt"
	"io"
	"net/rpc"
	"sync"
)

type serverCodec struct {
	mutex   sync.Mutex
	r       *bufio.Reader
	w       *bufio.Writer
	c       io.Closer
	pending map[uint64]uint64
	req     *Request
	seq     uint64
}

func NewServerCodec(rwc io.ReadWriteCloser) rpc.ServerCodec {
	return &serverCodec{
		r:       bufio.NewReader(rwc),
		w:       bufio.NewWriter(rwc),
		c:       rwc,
		pending: make(map[uint64]uint64),
	}
}

func (this *serverCodec) ReadRequestHeader(rpcreq *rpc.Request) error {
	f, err := read(this.r)
	if err != nil {
		return err
	}

	req := &Request{}
	if err := proto.Unmarshal(f, req); err != nil {
		return err
	}

	rpcreq.ServiceMethod = req.GetMethod()

	this.mutex.Lock()
	rpcreq.Seq = this.seq
	this.seq++
	this.req = req
	this.pending[rpcreq.Seq] = req.GetId()
	this.mutex.Unlock()

	return nil
}

func (this *serverCodec) ReadRequestBody(value interface{}) error {
	if value == nil {
		return nil
	}

	if msg, ok := value.(proto.Message); ok {
		if err := proto.Unmarshal(this.req.GetBody(), msg); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("unmarshal request body error: %s", value)
	}

	return nil
}

func (this *serverCodec) WriteResponse(rpcres *rpc.Response, value interface{}) error {
	this.mutex.Lock()
	id, ok := this.pending[rpcres.Seq]
	if !ok {
		this.mutex.Unlock()
		return errors.New("invalid sequence number in response")
	}
	delete(this.pending, rpcres.Seq)
	this.mutex.Unlock()

	res := &Response{}
	res.Id = proto.Uint64(id)

	if rpcres.Error == "" {
		if msg, ok := value.(proto.Message); ok {
			body, err := proto.Marshal(msg)
			if err != nil {
				return err
			}
			res.Body = body
		} else {
			return fmt.Errorf("marshal response body error: %s", value)
		}
	} else {
		res.Error = proto.String(rpcres.Error)
	}

	f, err := proto.Marshal(res)
	if err != nil {
		return err
	}

	if err := write(this.w, f); err != nil {
		return err
	}

	return nil
}

func (this *serverCodec) Close() error {
	return this.c.Close()
}

func ServeConn(conn io.ReadWriteCloser) {
	rpc.ServeCodec(NewServerCodec(conn))
}
