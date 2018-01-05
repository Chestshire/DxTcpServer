package ServerBase

import (
	"net"
	"time"
	"unsafe"
	"bytes"
	"fmt"
	"encoding/binary"
	"github.com/suiyunonghen/DxCommonLib"
	"log"
)

type DxTcpClient struct {
	Clientcon  	DxNetConnection
	encoder		IConCoder
	OnRecvData	GOnRecvDataEvent
	OnSendHeart	GConnectEvent
	OnClientconnect	GConnectEvent
	OnClientDisConnected	GConnectEvent
	OnSendData	GOnSendDataEvent
	TimeOutSeconds	int32
	ClientLogger *log.Logger
	sendBuffer	*bytes.Buffer
	donechan		chan struct{}
}

func (client *DxTcpClient)Active() bool {
	return !client.Clientcon.UnActive()
}

func (client *DxTcpClient)Done()<-chan struct{}  {
	return  client.donechan
}

func (client *DxTcpClient)Connect(addr string)error {
	if client.Active(){
		client.Close()
	}
	if tcpAddr, err := net.ResolveTCPAddr("tcp4", addr);err == nil{
		if conn, err := net.DialTCP("tcp", nil, tcpAddr);err == nil{ //创建一个TCP连接:TCPConn
			client.donechan = make(chan struct{})
			client.Clientcon.unActive.Store(false)
			client.Clientcon.con = conn
			client.Clientcon.LoginTime = time.Now() //登录时间
			client.Clientcon.ConHandle = uint(uintptr(unsafe.Pointer(client)))
			client.Clientcon.conHost = client
			client.Clientcon.IsClientcon = true
			client.Clientcon.protocol = nil
			if client.encoder != nil{
				if protocol,ok := client.encoder.(IProtocol);ok{
					client.Clientcon.protocol = protocol
				}
			}
			client.HandleConnectEvent(&client.Clientcon)
			DxCommonLib.Post(&client.Clientcon)//连接开始执行接收消息和发送消息的处理线程
			return nil
		}else{
			return err
		}
	}else {
		return err
	}
}

func (client *DxTcpClient)AddRecvDataLen(datalen uint32){

}

func (client *DxTcpClient)AddSendDataLen(datalen uint32){

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
	if client.OnClientDisConnected != nil{
		client.OnClientDisConnected(con)
	}
}

func (client *DxTcpClient)HeartTimeOutSeconds() int32 {
	return client.TimeOutSeconds
}

func (client *DxTcpClient)Close()  {
	close(client.donechan)
	client.Clientcon.Close()
}

func (client *DxTcpClient)EnableHeartCheck()bool  {
	return  true
}

func (client *DxTcpClient)SendHeart(con *DxNetConnection)  {
	if client.Active() && client.OnSendHeart !=nil{
		client.OnSendHeart(con)
	}
}

func (client *DxTcpClient)HandleRecvEvent(con *DxNetConnection,recvData interface{})  {
	if client.OnRecvData!=nil{
		client.OnRecvData(con,recvData)
	}
}

//设置编码解码器
func (client *DxTcpClient)SetCoder(encoder IConCoder)  {
	client.Close()
	client.encoder = encoder
}

func (client *DxTcpClient)GetCoder() IConCoder {
	return client.encoder
}


func (client *DxTcpClient)doOnSendData(params ...interface{})  {
	client.OnSendData(params[0].(*DxNetConnection),params[1],params[2].(int),params[3].(bool))
}

func (client *DxTcpClient)SendData(con *DxNetConnection,DataObj interface{})bool{
	if !client.Active(){
		return false
	}
	sendok := false
	var haswrite int = 0
	if con.protocol == nil && client.encoder!=nil{
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
			sendok = con.writeBytes(retbytes)
			con.LastValidTime = time.Now()
		}
		client.sendBuffer.Reset()
		if client.sendBuffer.Cap() > int(client.encoder.MaxBufferLen()){
			client.sendBuffer = nil //超过最大数据长度，就清理掉本次的
		}
	}else if con.protocol != nil{
		if client.sendBuffer == nil{
			client.sendBuffer = bytes.NewBuffer(make([]byte,0,client.encoder.MaxBufferLen()))
		}
		if retbytes,err := con.protocol.PacketObject(DataObj,client.sendBuffer);err==nil{
			sendok = con.writeBytes(retbytes)
		}else{
			sendok = false
			if client.ClientLogger != nil{
				client.ClientLogger.SetPrefix("[Error]")
				client.ClientLogger.Println(fmt.Sprintf("协议打包失败：%s",err.Error()))
			}
		}
	}
	if client.OnSendData != nil{
		DxCommonLib.PostFunc(client.doOnSendData,con,DataObj,haswrite,sendok)
	}
	return sendok
}
