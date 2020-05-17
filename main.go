package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/Unknwon/goconfig"

	"log"

	"time"

	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"github.com/tarm/goserial"
)

var queue []string
var cmd string
var COM_Title string
var COM_Name string
var COM_Baud int
var COM_Status int

var sendlist []string

func show() {
	if err := ui.Init(); err != nil {
		log.Fatalf("failed to initialize termui: %v", err)
	}
	defer ui.Close()

	p := widgets.NewParagraph()
	p.Title = "端口状态"
	p.Text = COM_Title
	p.SetRect(0, 3, 120, 6)
	p.TextStyle.Fg = ui.ColorWhite
	p.BorderStyle.Fg = ui.ColorCyan

	p0 := widgets.NewParagraph()
	p0.Title = "送片机协议调试"
	p0.Text = "[1]:初始化 [2]:复位/暂停 [3]:片盒状态 [401-460]:取片 [5]:还片 [6]:当前片盒 [7]:切换片盒"
	p0.SetRect(0, 0, 120, 3)
	p0.TextStyle.Fg = ui.ColorWhite
	p0.BorderStyle.Fg = ui.ColorCyan

	p1 := widgets.NewParagraph()
	p1.Title = "指令-编辑区"
	p1.Text = ":"
	p1.SetRect(0, 6, 120, 9)
	p1.TextStyle.Fg = ui.ColorYellow
	p1.BorderStyle.Fg = ui.ColorCyan

	l := widgets.NewList()
	l.Title = "操作-显示区"
	l.Rows = queue
	l.SetRect(0, 9, 120, 30)
	l.TextStyle.Fg = ui.ColorWhite
	l.BorderStyle.Fg = ui.ColorCyan

	draw := func() {
		if COM_Status == 1 {
			p.TextStyle.Fg = ui.ColorGreen
		} else {
			p.TextStyle.Fg = ui.ColorRed
		}
		l.Rows = queue
		p1.Text = ": " + cmd
		ui.Render(p, p0, p1, l)
	}

	draw()

	uiEvents := ui.PollEvents()
	ticker := time.NewTicker(time.Second).C
	for {
		select {
		case e := <-uiEvents:
			// print(e.ID)
			switch e.ID {
			// case "q", "<C-c>":
			// 	return
			case "<Enter>":
				n, err := strconv.Atoi(cmd)
				if err != nil {
					print(">>>> " + cmd + " 错误的指令")
				} else {
					sent(n)
				}
				cmd = ""
				draw()
			case "<C-<Backspace>>":
				pop_cmd()
				draw()
			default:
				if len(e.ID) == 1 {
					push_cmd(e.ID)
					draw()
				}
			}
		case <-ticker:
			// print(time.Now().String())
			draw()
		}
	}
}

func com() {
	cfg, err := goconfig.LoadConfigFile("conf.ini")
	if err != nil {
		COM_Title = "配置文件不存在!"
		return
	}
	Name, err := cfg.GetValue("Com", "Name")
	if err != nil {
		COM_Title = "参数错误!"
		return
	}
	COM_Name = Name

	Baud, err := cfg.Int("Com", "Baud")
	if err != nil {
		COM_Title = "参数错误!"
		return
	}
	COM_Baud = Baud

	title := fmt.Sprintf("端口：%s  波特率：%d", COM_Name, COM_Baud)

	c := &serial.Config{Name: Name, Baud: Baud}

	s, err := serial.OpenPort(c)
	if err != nil {
		COM_Status = 0
		COM_Title = title + "  状态：通讯异常"
		return
	}

	COM_Status = 1
	COM_Title = title + "  状态：通讯正常"

	defer s.Close()

	go sender(s)
	go receiver(s)

	for {
		time.Sleep(1)
	}
	return
}

func sent(n int) {
	header := "90eb"
	message := ""
	title := ""
	if n == 1 {
		message = "040001"
		title = " 上位机 发送 [初始化] 指令"
	}
	if n == 2 {
		message = "040002"
		title = " 上位机 发送 [复位/暂停] 指令"
	}
	if n == 3 {
		message = "040003"
		title = " 上位机 发送 [获取片盒状态] 指令"
	}
	if n > 400 && n < 461 {
		message = "050004"
		x := n - 400
		m := strconv.FormatInt(int64(x), 16)
		if len(m) == 1 {
			m = "0" + m
		}
		message = message + m
		title = " 上位机 发送 [取片 " + strconv.Itoa(x) + "] 指令"
	}
	if n == 5 {
		message = "040005"
		title = " 上位机 发送 [还片] 指令"
	}
	if n == 6 {
		message = "040006"
		title = " 上位机 发送 [获取当前片盒] 指令"
	}
	if n == 7 {
		message = "040007"
		title = " 上位机 发送 [切换片盒] 指令"
	}
	if len(message) > 0 {
		data, _ := hex.DecodeString(message)
		crc := hex.EncodeToString(IntToBytes(CRC16_IBM(data, len(data))))
		sendlist = append(sendlist, header+message+crc)
		print(">>>> " + strings.ToUpper(header+message+crc+title))
	}
}

func sender(s io.ReadWriteCloser) {
	for {
		if len(sendlist) > 0 {
			buf, _ := hex.DecodeString(sendlist[0])
			_, e := s.Write(buf)
			if e != nil {
				print(e.Error())
				return
			}
			sendlist = sendlist[1:]
		}
		time.Sleep(1)
	}
}

func receiver(s io.ReadWriteCloser) {
	buf := make([]byte, 128)
	var buffer string
	for {
		n, e := s.Read(buf)
		if e != nil {
			print(e.Error())
			break
		}
		if n > 0 {
			buffer += hex.EncodeToString(buf[:n])
			buffer = unpack(buffer)
		}
	}
}

func hexformat(buf []byte) string {
	s := hex.EncodeToString(buf)
	return s
}

func parse(s string) string {
	d, _ := hex.DecodeString(s)
	msg := ""
	if d[3] == 0x01 {
		msg = "送片机"
	}
	if d[4] == 0x01 {
		if d[5] == 0x00 {
			msg += " 执行 [初始化] 操作成功"
		}
		if d[5] == 0x01 {
			msg += " 执行 [初始化] 操作失败"
		}
		if d[5] == 0x02 {
			msg += " 收到 [初始化] 指令"
		}
	}
	if d[4] == 0x02 {
		if d[5] == 0x00 {
			msg += " 执行 [复位/暂停] 操作成功"
		}
		if d[5] == 0x01 {
			msg += " 执行 [复位/暂停] 操作失败"
		}
		if d[5] == 0x02 {
			msg += " 收到 [复位/暂停] 指令"
		}
	}
	if d[4] == 0x03 {
		if d[5] == 0x00 {
			msg += " 执行 [获取片盒状态] 操作成功"
		}
		if d[5] == 0x01 {
			msg += " 执行 [获取片盒状态] 操作失败"
		}
		if d[5] == 0x02 {
			msg += " 收到 [获取片盒状态] 指令"
		}
	}
	if d[4] == 0x04 {
		if d[5] == 0x00 {
			msg += " 执行 [取片] 操作成功"
		}
		if d[5] == 0x01 {
			msg += " 执行 [取片] 操作失败"
		}
		if d[5] == 0x02 {
			msg += " 收到 [取片] 指令"
		}
	}
	if d[4] == 0x05 {
		if d[5] == 0x00 {
			msg += " 执行 [还片] 操作成功"
		}
		if d[5] == 0x01 {
			msg += " 执行 [还片] 操作失败"
		}
		if d[5] == 0x02 {
			msg += " 收到 [还片] 指令"
		}
	}
	if d[4] == 0x06 {
		if d[5] == 0x00 {
			msg += " 执行 [获取当前片盒] 操作成功"
		}
		if d[5] == 0x01 {
			msg += " 执行 [获取当前片盒] 操作失败"
		}
		if d[5] == 0x02 {
			msg += " 收到 [获取当前片盒] 指令"
		}
	}
	if d[4] == 0x07 {
		if d[5] == 0x00 {
			msg += " 执行 [切换片盒] 操作成功"
		}
		if d[5] == 0x01 {
			msg += " 执行 [切换片盒] 操作失败"
		}
		if d[5] == 0x02 {
			msg += " 收到 [切换片盒] 指令"
		}
	}
	return msg
}

// 组包、拆包处理
func unpack(s string) string {
	s = strings.ToUpper(s)
	head := "90EB"
	if strings.Index(s, head) < 0 {
		return s
	}
	s = strings.ReplaceAll(s, head, " "+head)
	s = strings.TrimSpace(s)
	data := strings.Split(s, " ")
	s = ""
	i := 0
	for i < len(data) {
		if check(data[i]) {
			print("<<<< " + data[i] + " " + parse(data[i]))
		} else {
			if i == len(data)-1 {
				s = data[i]
			}
		}
		i++
	}
	return s
}

func check(s string) bool {
	if strings.Index(s, "90EB") == 0 && len(s) > 8 {
		buf, _ := hex.DecodeString(s[4 : len(s)-4])
		crc := CRC16_IBM(buf, 4)
		crc_string := hex.EncodeToString(IntToBytes(crc))
		crc_string = strings.ToUpper(crc_string)
		if strings.Contains(crc_string, s[len(s)-4:]) {
			return true
		}
	}
	return false
}

func IntToBytes(n int) []byte {
	x := int16(n)
	bytesBuffer := bytes.NewBuffer([]byte{})
	binary.Write(bytesBuffer, binary.BigEndian, x)
	return bytesBuffer.Bytes()
}

func CRC16_IBM(data []byte, datalen int) int {
	wCRCin := 0x0000
	wCPoly := 0xA001
	n := 0
	for n < datalen {
		wCRCin = wCRCin ^ int(data[n])
		for i := 0; i < 8; i++ {
			if wCRCin&0x01 > 0 {
				wCRCin = (wCRCin >> 1) ^ wCPoly
			} else {
				wCRCin = wCRCin >> 1
			}
		}
		n++
	}
	return wCRCin<<8 | wCRCin>>8
}

func push_cmd(s string) {
	cmd += s
}

func pop_cmd() {
	if len(cmd) > 0 {
		cmd = cmd[:len(cmd)-1]
	}
}

func print(s string) {
	queue = append(queue, time.Now().Format("2006-01-02 15:04:05")+" : "+s)
	if len(queue) > 19 {
		queue = queue[1:]
	}
}

func main() {
	go com()
	go show()
	for {
		time.Sleep(1)
	}
}
