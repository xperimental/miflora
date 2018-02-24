package miflora

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"os/exec"
	"strings"
)

type Miflora struct {
	mac      string
	adapter  string
	firmware Firmware
}

func NewMiflora(mac string, adapter string) *Miflora {
	return &Miflora{
		mac:     mac,
		adapter: adapter,
	}
}

func gattCharRead(mac string, handle string, adapter string) ([]byte, error) {
	cmd := exec.Command("gatttool", "-b", mac, "--char-read", "-a", handle, "-i", adapter)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	// Characteristic value/descriptor: 64 10 32 2e 36 2e 32
	s := out.String()
	if !strings.HasPrefix(s, "Characteristic value/descriptor: ") {
		return nil, errors.New("Unexpected response")
	}

	// Decode the hex bytes
	r := strings.NewReplacer(" ", "", "\n", "")
	s = r.Replace(s[33:])
	h, err := hex.DecodeString(s)
	if err != nil {
		return nil, err
	}
	return h, nil
}

func gattCharWrite(mac string, handle string, value string, adapter string) error {
	cmd := exec.Command("gatttool", "-b", mac, "--char-write-req", "-a", handle, "-n", value, "-i", adapter)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return err
	}

	s := out.String()
	if !strings.Contains(s, "successfully") {
		return errors.New("Unexpected response")
	}

	return nil
}

type Firmware struct {
	Version string
	Battery byte
}

func (m *Miflora) ReadFirmware() (Firmware, error) {
	data, err := gattCharRead(m.mac, "0x38", m.adapter)
	if err != nil {
		return Firmware{}, err
	}
	f := Firmware{
		Version: string(data[2:]),
		Battery: data[0],
	}
	m.firmware = f
	return f, nil
}

type Sensors struct {
	Temperature  float64
	Moisture     byte
	Light        uint16
	Conductivity uint16
}

func (m *Miflora) enableRealtimeDataReading() error {
	return gattCharWrite(m.mac, "0x33", "A01F", m.adapter)
}

func decodeSensors(data []byte) Sensors {
	p := bytes.NewBuffer(data)
	var t int16
	var m uint8
	var l, c uint16

	// TT TT ?? LL LL ?? ?? MM CC CC
	binary.Read(p, binary.LittleEndian, &t)
	p.Next(1)
	binary.Read(p, binary.LittleEndian, &l)
	p.Next(2)
	binary.Read(p, binary.LittleEndian, &m)
	binary.Read(p, binary.LittleEndian, &c)

	return Sensors{
		Temperature:  float64(t) / 10,
		Moisture:     m,
		Light:        l,
		Conductivity: c,
	}
}

func (m *Miflora) ReadSensors() (Sensors, error) {
	if m.firmware.Version >= "2.6.6" {
		// newer firmwares explicitly need realtime reading enabling
		err := m.enableRealtimeDataReading()
		if err != nil {
			return Sensors{}, err
		}
	}

	data, err := gattCharRead(m.mac, "0x35", m.adapter)
	if err != nil {
		return Sensors{}, err
	}
	return decodeSensors(data), nil
}
