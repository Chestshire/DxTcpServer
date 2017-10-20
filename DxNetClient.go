package DxTcpServer

import (
	"net"
	"time"
	"unsafe"
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/landjur/golibrary/log"
)

type DxTcpClient struct {
	Clientcon  	DxNetConnection
	encoder		IConCoder
	OnRecvData	GOnRecvDataEvent
	OnSendHeart	GConnectEvent
	OnClientconnect	GConnectEvent
	OnClientDisConnected	GConnectEvent
	OnSendData	GOnSendDataEvent
	Active		bool
	TimeOutSeconds	int32
	ClientLogger *log.Logger
	sendBuffer	*bytes.Buffer
}

func (client *DxTcpClient)Connect(addr string)error {
	if client.Active{
		client.Close()
	}
	if tcpAddr, err := net.ResolveTCPAddr("tcp4", addr);err == nil{
		if conn, err := net.DialTCP("tcp", nil, tcpAddr);err == nil{ //创建一个TCP连接:TCPConn
			client.Clientcon.unActive = false
			client.Clientcon.con = conn
			client.Clientcon.LoginTime = time.Now() //登录时间
			client.Clientcon.ConHandle = uint(uintptr(unsafe.Pointer(client)))
			client.Clientcon.conHost = client
			client.Clientcon.IsClientcon = true
			client.HandleConnectEvent(&client.Clientcon)
			client.Clientcon.run() //连接开始执行接收消息和发送消息的处理线程
			client.Active = true
			return nil
		}else{
			return err
		}
	}else {
		return err
	}
}

func (client *DxTcpClient)Logger()*log.Logger  {
	return client.ClientLogger
}

func (client *DxTcpClient)HandleConnectEvent(con *DxNetConnection)  {
	if client.OnClientconnect!=nil{
		client.OnClientconnect(con)
	}
}

func (client *DxTcpClient)HandleDisConnectEvent(con *DxNetConnection) {
	client.Active = false
	if client.OnClientDisConnected != nil{
		client.OnClientDisConnected(con)
	}
}

func (client *DxTcpClient)HeartTimeOutSeconds() int32 {
	return client.TimeOutSeconds
}

func (client *DxTcpClient)Close()  {
	if client.Active{
		client.Clientcon.Close()
		client.Active = false
	}
}

func (client *DxTcpClient)EnableHeartCheck()bool  {
	return  true
}

func (client *DxTcpClient)SendHeart(con *DxNetConnection)  {
	if client.Active && client.OnSendHeart !=nil{
		client.OnSendHeart(con)
	}
}

func (client *DxTcpClient)HandleRecvEvent(con *DxNetConnection,recvData interface{},recvDataLen uint32)  {
	if client.OnRecvData!=nil{
		client.OnRecvData(con,recvData)
	}
}

//设置编码解码器
func (client *DxTcpClient)SetCoder(encoder IConCoder)  {
	if client.Active{
		client.Close()
	}
	client.encoder = encoder
}

func (client *DxTcpClient)GetCoder() IConCoder {
	return client.encoder
}

func (client *DxTcpClient)SendData(con *DxNetConnection,DataObj interface{})bool{
	if !client.Active || con.unActive{
		return false
	}
	sendok := false
	var haswrite int = 0
	if client.encoder!=nil{
		var retbytes []byte
		if client.sendBuffer == nil{
			client.sendBuffer = bytes.NewBuffer(make([]byte,0,client.encoder.MaxBufferLen()))
		}
		headLen := client.encoder.HeadBufferLen()
		if headLen > 2{
			headLen = 4
		}else{
			headLen = 2
		}
		//先写入数据内容长度进去
		if headLen <= 2{
			binary.Write(client.sendBuffer,binary.LittleEndian,uint16(1))
		}else{
			binary.Write(client.sendBuffer,binary.LittleEndian,uint32(1))
		}
		if err := client.encoder.Encode(DataObj,client.sendBuffer);err==nil{
			retbytes = client.sendBuffer.Bytes()
			lenb := len(retbytes)
			objbuflen := lenb-int(headLen)
			//然后写入实际长度
			if headLen <= 2{
				if client.encoder.UseLitterEndian(){
					binary.LittleEndian.PutUint16(retbytes[0:headLen],uint16(objbuflen))
				}else{
					binary.BigEndian.PutUint16(retbytes[0:headLen],uint16(objbuflen))
				}
			}else{
				if client.encoder.UseLitterEndian(){
					binary.LittleEndian.PutUint32(retbytes[0:headLen],uint32(objbuflen))
				}else{
					binary.BigEndian.PutUint32(retbytes[0:headLen],uint32(objbuflen))
				}
			}
			for {
				con.LastValidTime = time.Now()
				if wln,err := con.con.Write(retbytes[haswrite:lenb]);err != nil{
					if client.ClientLogger != nil{
						client.ClientLogger.SetPrefix("[Error]")
						client.ClientLogger.Debugln(fmt.Sprintf("写入远程客户端%s失败，程序准备断开：%s",con.RemoteAddr(),err.Error()))
					}
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
			con.SendDataLen.AddByteSize(uint32(lenb))
		}
		client.sendBuffer.Reset()
		if client.sendBuffer.Cap() > int(client.encoder.MaxBufferLen()){
			client.sendBuffer = nil //超过最大数据长度，就清理掉本次的
		}
	}
	if client.OnSendData != nil{
		client.OnSendData(con,DataObj,haswrite,sendok)
	}
	return sendok
}
