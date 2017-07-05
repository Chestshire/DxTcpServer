package DxTcpServer

import (
	"net"
	"time"
	"sync/atomic"
	"unsafe"
	"bytes"
	"encoding/binary"
	"sync"
	"fmt"
)


type DxDiskSize struct {
	SizeByte	uint16
	SizeKB		uint16
	SizeMB		uint16
	SizeGB		uint16
	SizeTB		uint32
}

func (size *DxDiskSize)Add(nsize *DxDiskSize)  {
	var tmp uint32 = uint32(size.SizeByte + nsize.SizeByte)
	var reallen = tmp / 1024
	size.SizeByte = uint16(tmp % 1024)

	tmp = uint32(size.SizeKB+nsize.SizeKB) + reallen
	reallen = tmp / 1024
	size.SizeKB = uint16(tmp % 1024)


	tmp = uint32(size.SizeMB + nsize.SizeMB) + reallen
	reallen = tmp / 1024
	size.SizeMB = uint16(tmp % 1024)

	tmp = uint32(size.SizeGB + nsize.SizeGB) + reallen
	reallen = tmp / 1024
	size.SizeGB = uint16(tmp % 1024)

	size.SizeTB = uint32(size.SizeTB + nsize.SizeTB) + reallen
}

func (size *DxDiskSize)AddByteSize(ByteSize uint32)  {
	var tmp uint32 = uint32(size.SizeByte) + ByteSize
	var reallen = tmp / 1024
	size.SizeByte = uint16(tmp % 1024)
	if reallen == 0{
		return
	}
	tmp = uint32(size.SizeKB) + reallen
	reallen = tmp / 1024
	size.SizeKB = uint16(tmp % 1024)
	if reallen == 0{
		return
	}

	tmp = uint32(size.SizeMB) + reallen
	reallen = tmp / 1024
	size.SizeMB = uint16(tmp % 1024)
	if reallen == 0{
		return
	}

	tmp = uint32(size.SizeGB) + reallen
	reallen = tmp / 1024
	size.SizeGB = uint16(tmp % 1024)
	if reallen == 0{
		return
	}

	size.SizeTB = uint32(size.SizeTB) + reallen
}

func (size *DxDiskSize)ToString(useHtmlTag bool)(result string)  {
	fmtstr := "%d"
	if useHtmlTag{
		fmtstr = `<font color="blue"><b>%d</b></font>%s`
	}
	if size.SizeTB >0{
		result = fmt.Sprintf(fmtstr,size.SizeTB,"TB ")
	}else{
		result = ""
	}
	if useHtmlTag{
		fmtstr = `%s<font color="blue"><b>%d</b></font>%s`
	}else{
		fmtstr = "%s%d%s"
	}
	if size.SizeGB > 0{
		result = fmt.Sprintf(fmtstr,result,size.SizeGB,"GB ")
	}
	if size.SizeMB > 0{
		result = fmt.Sprintf(fmtstr,result,size.SizeMB,"MB ")
	}
	if size.SizeKB > 0{
		result = fmt.Sprintf(fmtstr,result,size.SizeKB,"KB ")
	}
	if size.SizeByte > 0{
		result = fmt.Sprintf(fmtstr,result,size.SizeByte,"Byte ")
	}
	return
}

type DxTcpServer struct {
	listener        net.Listener
	encoder		IConCoder
	isActivetag	int32
	OnRecvData	GOnRecvDataEvent
	OnClientConnect	GConnectEvent
	OnClientDisConnected	GConnectEvent
	OnSendData	GOnSendDataEvent
	TimeOutSeconds	int32
	curidx		uint
	clients 	map[uint]*DxNetConnection
	RequestCount	uint64
	SendRequestCount	uint64
	LimitSendPkgCount	uint8  //每个连接限制的发送包的个数，防止发送过快
	SendDataSize	 DxDiskSize
	RecvDataSize	DxDiskSize
	MaxDataBufCount	uint16		//最大缓存数量
	dataBuffer	chan[]byte   //缓存列表
	sync.RWMutex
}
type GIterateClientFunc func(con *DxNetConnection)
func (srv *DxTcpServer)Open(addr string) error {
	ls, err := net.Listen("tcp", addr)
	if nil != err {
		return err
	}
	srv.listener = ls
	srv.isActivetag = 1
	go srv.acceptClients()
	return nil
}


func (srv *DxTcpServer)GetCoder()IConCoder{
	return srv.encoder
}


func (srv *DxTcpServer)Close()  {
	if !atomic.CompareAndSwapInt32(&srv.isActivetag,1,0){
		return
	}
	if nil != srv.listener {
		srv.listener.Close()
	}
	//fmt.Println("还剩下",len(srv.clients))
	for k, c := range srv.clients {
		c.Close()
		delete(srv.clients, k)
	}
}


func (srv *DxTcpServer)acceptClients()  {
	for{
		conn, err := srv.listener.Accept()
		if err != nil {
			srv.listener = nil
			return
		}
		dxcon := new(DxNetConnection)
		dxcon.con = conn
		dxcon.LimitSendPkgCout = srv.LimitSendPkgCount
		dxcon.LoginTime = time.Now() //登录时间
		dxcon.ConHandle = uint(uintptr(unsafe.Pointer(dxcon)))
		dxcon.conHost = srv
		if srv.clients == nil{
			srv.clients = make(map[uint]*DxNetConnection)
		}
		srv.Lock()
		srv.clients[dxcon.ConHandle] = dxcon
		srv.Unlock()
		srv.HandleConnectEvent(dxcon)
		dxcon.run() //连接开始执行接收消息和发送消息的处理线程
	}
}

func (srv *DxTcpServer)HandleDisConnectEvent(con *DxNetConnection) {
	if srv.OnClientDisConnected != nil{
		srv.OnClientDisConnected(con)
	}
	srv.Lock()
	con.SetUseData(nil)
	delete(srv.clients,con.ConHandle)
	srv.Unlock()
}

func (srv *DxTcpServer)SendHeart(con *DxNetConnection)  {

}

func (srv *DxTcpServer)GetBuffer()(retbuf []byte)  {
	var ok bool
	if srv.dataBuffer != nil{
		select{
		case retbuf,ok = <-srv.dataBuffer:
			if !ok{
				retbuf = make([]byte,srv.encoder.MaxBufferLen())
			}
		default:
			retbuf = make([]byte,srv.encoder.MaxBufferLen())
		}
	}else if srv.dataBuffer == nil && srv.MaxDataBufCount != 0{
		srv.dataBuffer = make(chan []byte,srv.MaxDataBufCount)
		retbuf = make([]byte,srv.encoder.MaxBufferLen())
	}else{
		retbuf = make([]byte,srv.encoder.MaxBufferLen())
	}
	return
}

func (srv *DxTcpServer)ReciveBuffer(buf []byte)bool  {
	if srv.dataBuffer != nil{
		select{
		case srv.dataBuffer <- buf:
			return true
		case <-After(time.Second * 5):
			//回收失败
			return false
		}
	}
	return false
}

func (srv *DxTcpServer)SendData(con *DxNetConnection,DataObj interface{})bool  {
	coder := srv.encoder
	sendok := false
	var haswrite int = 0
	if coder!=nil{
		var retbytes []byte
		sendBuffer := srv.GetBuffer()
		headLen := coder.HeadBufferLen()
		retbytes = sendBuffer[0:headLen]
		buf := bytes.NewBuffer(retbytes[:headLen])
		if err := coder.Encode(DataObj,buf);err==nil{
			retbytes = buf.Bytes()
			objbuflen := uint16(buf.Len()) - headLen
			binary.BigEndian.PutUint16(retbytes[0:headLen],uint16(objbuflen))
			lenb := len(retbytes)
			buf = nil
			for {
				con.LastValidTime = time.Now()
				if wln,err := con.con.Write(retbytes[haswrite:lenb]);err != nil{
					con.Close()
					break
				}else{
					haswrite+=wln
					if haswrite == lenb{
						sendok =true
						break
					}
				}
			}
			//写入发送了多少数据
			con.LastValidTime = time.Now()
			srv.Lock()
			srv.SendDataSize.AddByteSize(uint32(lenb))
			srv.Unlock()
		}
		srv.ReciveBuffer(sendBuffer)//回收
		sendBuffer = nil
	}
	if srv.OnSendData != nil{
		srv.OnSendData(con,DataObj,haswrite,sendok)
	}
	if sendok{
		atomic.AddUint64(&srv.SendRequestCount,1) //增加回复的请求数量
	}
	return sendok
}

func (srv *DxTcpServer)HandleConnectEvent(con *DxNetConnection)  {
	if srv.OnClientConnect!=nil{
		srv.OnClientConnect(con)
	}
}

func (srv *DxTcpServer)EnableHeartCheck() bool {
	return true
}

func (srv *DxTcpServer)ClientCount()(result int) {
	srv.Lock()
	result = len(srv.clients)
	srv.Unlock()
	return
}

//遍历客户端数据连接
func (srv *DxTcpServer)ClientIterate(iteratefunc GIterateClientFunc)  {
	srv.Lock()
	for _, c := range srv.clients {
		iteratefunc(c)
	}
	srv.Unlock()
}

//获取所有客户端map
func (srv *DxTcpServer)GetClients()map[uint]*DxNetConnection{
	return srv.clients
}


func (srv *DxTcpServer)HandleRecvEvent(con *DxNetConnection,recvData interface{},recvDataLen uint16)  {
	atomic.AddUint64(&srv.RequestCount,1) //增加接收的请求数量
	srv.Lock()
	srv.RecvDataSize.AddByteSize(uint32(recvDataLen))
	srv.Unlock()
	//增加接收的数量信息
	con.ReciveDataLen.AddByteSize(uint32(recvDataLen))
	if srv.OnRecvData!=nil{
		srv.OnRecvData(con,recvData)
	}
}

func (srv *DxTcpServer)HeartTimeOutSeconds() int32 {
	return srv.TimeOutSeconds
}
//设置编码解码器
func (srv *DxTcpServer)SetCoder(encoder IConCoder)  {
	if srv.Active(){
		srv.Close()
	}
	srv.encoder = encoder
}

func (srv *DxTcpServer)Active()bool  {
	activeflag := atomic.LoadInt32(&srv.isActivetag)
	return activeflag != 0
}
