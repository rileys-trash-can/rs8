package nt8

import (
	"bufio"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/tarm/serial"
	"io"
	"log"
	"strings"
	"time"
)

type Connection struct {
	*serial.Port

	LightCmdCh chan<- []CmdLight
}

func Open(name string) (*Connection, error) {
	p, err := serial.OpenPort(&serial.Config{
		Name: name,
		Baud: 9600,
	})
	if err != nil {
		return nil, err
	}

	c := &Connection{p, nil}
	go c.handleCmds()

	return c, nil
}

type Event interface {
	event()
}

type EventButton struct {
	Type      EventButtonType
	Direction Direction

	Value uint8
}

type Direction uint8

const (
	Down Direction = iota
	Up
)

func (*EventButton) event() {}

type EventButtonType uint8

const (
	ButtonProgram     EventButtonType = 0b0010
	ButtonPreview                     = 0b0001
	ButtonAutoTakeDSK                 = 0b0011
)

type EventSlider struct {
	Type EventSliderType

	Value uint8
}

func (*EventSlider) event() {}

type EventSliderType uint8

const (
	SliderTbar EventSliderType = iota
	RotA
	RotB
	RotC
)

type CmdLight struct {
	Type  EventButtonType
	Value uint8

	State Light
}

type Light uint8

const (
	LightOn Light = iota
	LightOff
)

// first nibble of first byte is ignored
func (c *Connection) WriteCmd(cmd [2]byte) (err error) {
	h := strings.ToUpper(hex.EncodeToString(cmd[:]))

	log.Printf("sending: ~%s\r", h[1:])
	_, err = fmt.Fprintf(c.Port, "~%s\r", h[1:])
	return
}

func (c *Connection) handleCmds() {
	ch := make(chan []CmdLight)
	c.LightCmdCh = ch

	ledmap := make(map[EventButtonType]uint8)
	ledmap[ButtonProgram] = 0xFF
	ledmap[ButtonPreview] = 0xFF
	ledmap[ButtonAutoTakeDSK] = 0xFF

	for {
		changed := make(map[EventButtonType]bool)

		cmds := <-ch
		//log.Printf("got cmds:  %d %#v", len(cmds), nil)

		for _, cmd := range cmds {
			var (
				oldstate = T(ledmap[cmd.Type]&(1<<cmd.Value) == 0, LightOn, LightOff)
				state    = cmd.State
			)

			// log.Printf("oldstate: %01b state %01b", oldstate, state)

			if oldstate != state {
				ledmap[cmd.Type] ^= (1 << cmd.Value)
				changed[cmd.Type] = true
				// send updated state:
			}
		}

		for t := range changed {
			err := c.WriteCmd([2]byte{
				byte(t),
				ledmap[t],
			})
			if err != nil {
				log.Printf("Failed to wrtie CMD: %s", err)
			}

		}
	}
}

// does fancy blinky blinky sequence
func (conn *Connection) InitBlynk() {
	cmd := CmdLight{}
	c := time.NewTicker(time.Second / 16)

	for i := uint8(0); i < 8; i++ {
		<-c.C
		cmd.Type = ButtonProgram
		cmd.Value = 7 - i
		cmd.State = LightOn

		conn.LightCmdCh <- []CmdLight{cmd}
	}

	for i := uint8(0); i < 8; i++ {
		<-c.C
		cmd.Type = ButtonPreview
		cmd.Value = 7 - i
		cmd.State = LightOn

		conn.LightCmdCh <- []CmdLight{cmd}
	}

	for i := uint8(0); i < 4; i++ {
		<-c.C
		cmd.Type = ButtonAutoTakeDSK
		cmd.Value = 7 - i
		cmd.State = LightOn

		conn.LightCmdCh <- []CmdLight{cmd}
	}

	for i := uint8(0); i < 3; i++ {
		<-c.C
		cmd.Type = ButtonAutoTakeDSK
		cmd.Value = i
		cmd.State = LightOn

		conn.LightCmdCh <- []CmdLight{cmd}
	}

	// off
	for i := uint8(0); i < 8; i++ {
		<-c.C
		cmd.Type = ButtonProgram
		cmd.Value = 7 - i
		cmd.State = LightOff

		conn.LightCmdCh <- []CmdLight{cmd}
	}

	for i := uint8(0); i < 8; i++ {
		<-c.C
		cmd.Type = ButtonPreview
		cmd.Value = 7 - i
		cmd.State = LightOff

		conn.LightCmdCh <- []CmdLight{cmd}
	}

	for i := uint8(0); i < 4; i++ {
		<-c.C
		cmd.Type = ButtonAutoTakeDSK
		cmd.Value = 7 - i
		cmd.State = LightOff

		conn.LightCmdCh <- []CmdLight{cmd}
	}

	for i := uint8(0); i < 3; i++ {
		<-c.C
		cmd.Type = ButtonAutoTakeDSK
		cmd.Value = i
		cmd.State = LightOff

		conn.LightCmdCh <- []CmdLight{cmd}
	}

	c.Stop()

}

func (c *Connection) ReadCh() <-chan Event {
	ch := make(chan Event, 2)

	go func() {
		r := bufio.NewReader(c.Port)
		cmd := make([]byte, 2)

		var (
			program byte = 0xFF
			preview byte = 0xFF

			atdst byte = 0xFF
		)

		for {
			hcmd, err := r.ReadBytes('\r')
			if err != nil {
				log.Printf("Error reading: %s", err)
			}

			if errors.Is(err, io.EOF) {
				log.Printf("FATAL; EOF")
				return
			}

			if len(hcmd) != 5 {
				log.Printf("illigal command received: %02X %s %d", hcmd, hcmd, len(hcmd))
				continue
			}

			//log.Printf("%v %s", hcmd[1:4], hcmd[1:4])

			_, err = hex.Decode(cmd, append([]byte("0"), hcmd[1:4]...))
			if err != nil {
				log.Printf("Failed to decode hex command: %s", err)
				continue
			}

			//log.Printf("%08b %d", cmd, len(cmd))

			segment := cmd[0]
			//log.Printf("Segment: 0b %04b   0x %1X", segment, segment)

			switch segment {
			case 0b0010: // program
				for i := uint8(0); i < 8; i++ {
					mask := byte(1 << i)

					state := cmd[1] & mask
					oldstate := program & mask

					if state != oldstate {
						/*						log.Printf("mask: %08b", mask)
												log.Printf("sta:  %08b", state)
												log.Printf("osta: %08b", oldstate)
						*/

						program ^= mask // toggle oldstate

						ch <- &EventButton{
							Type:      ButtonProgram,
							Direction: T(state == 0, Down, Up),

							Value: i,
						}
					}
				}
			case 0b0001: // preview
				for i := uint8(0); i < 8; i++ {
					mask := byte(1 << i)

					state := cmd[1] & mask
					oldstate := preview & mask

					if state != oldstate {
						/*						log.Printf("mask: %08b", mask)
												log.Printf("sta:  %08b", state)
												log.Printf("osta: %08b", oldstate)
						*/
						preview ^= mask // toggle oldstate

						ch <- &EventButton{
							Type:      ButtonPreview,
							Direction: T(state == 0, Down, Up),

							Value: i,
						}
					}
				}
			case 0b0011: // atdsk
				for i := uint8(0); i < 8; i++ {
					mask := byte(1 << i)

					state := cmd[1] & mask
					oldstate := atdst & mask

					if state != oldstate {
						/*						log.Printf("mask: %08b", mask)
												log.Printf("sta:  %08b", state)
												log.Printf("osta: %08b", oldstate)
						*/

						atdst ^= mask // toggle oldstate

						ch <- &EventButton{
							Type:      ButtonAutoTakeDSK,
							Direction: T(state == 0, Down, Up),

							Value: i,
						}
					}
				}
			case 0b0100: // slider1
				ch <- &EventSlider{
					Type: SliderTbar,

					Value: cmd[1],
				}
			case 0b0101: // slider1
				ch <- &EventSlider{
					Type: RotA,

					Value: cmd[1],
				}
			case 0b0110: // slider1
				ch <- &EventSlider{
					Type: RotB,

					Value: cmd[1],
				}
			case 0b0111: // slider1
				ch <- &EventSlider{
					Type: RotC,

					Value: cmd[1],
				}
			}
		}
	}()

	return ch
}

func T[K any](c bool, a, b K) K {
	if c {
		return a
	}

	return b
}
