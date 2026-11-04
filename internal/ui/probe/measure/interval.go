package measure

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

// nominalInterval is the 60 Hz frame interval in milliseconds that v0.2.0's
// baseline was measured at.
const nominalInterval = 1000.0 / 60

// intervalTolerance is how far an interval may be from nominalInterval
// before the probe says so.
const intervalTolerance = 0.10

// AppID is the app's Wayland application id, which finds its window in the
// compositor's tree.
const AppID = "dev.coredev.Sona"

// IntervalWarning is the warning for a display interval that is not about
// 60 Hz (more than 10% from 16.67 ms), or "" when it is. source says where
// the value came from.
func IntervalWarning(ms float64, source string) string {
	if ms > 0 && ms >= nominalInterval*(1-intervalTolerance) && ms <= nominalInterval*(1+intervalTolerance) {
		return ""
	}
	return fmt.Sprintf("warning: the display interval is %.2f ms (%s), not 16.67 ms ±10%%: frame and dropped-frame figures are not comparable with the 60 Hz baseline", ms, source)
}

// swayTree and swayOutput are the parts of sway's IPC answers read here.
type swayNode struct {
	Type  string     `json:"type"`
	Name  string     `json:"name"`
	AppID string     `json:"app_id"`
	PID   int        `json:"pid"`
	Nodes []swayNode `json:"nodes"`
	Float []swayNode `json:"floating_nodes"`
}

type swayOutput struct {
	Name        string `json:"name"`
	Active      bool   `json:"active"`
	CurrentMode struct {
		Refresh int `json:"refresh"` // mHz
	} `json:"current_mode"`
}

// windowOutput is the name of the output the app's window is on: the
// output node of sway's tree that holds a window with the pid, else the
// app id. "" when there is none.
func windowOutput(tree swayNode, pid int, appID string) string {
	var find func(n swayNode, output string) (string, bool)
	find = func(n swayNode, output string) (string, bool) {
		if n.Type == "output" {
			output = n.Name
		}
		if output != "" && ((pid > 0 && n.PID == pid) || (pid <= 0 && appID != "" && n.AppID == appID)) {
			return output, true
		}
		for _, c := range append(append([]swayNode(nil), n.Nodes...), n.Float...) {
			if o, ok := find(c, output); ok {
				return o, true
			}
		}
		return "", false
	}
	o, _ := find(tree, "")
	return o
}

// outputInterval is the frame interval in milliseconds of the named output
// (the only active one when name is empty), from its mode's refresh.
func outputInterval(outputs []swayOutput, name string) (float64, error) {
	var active []swayOutput
	for _, o := range outputs {
		if o.Active {
			active = append(active, o)
		}
		if name != "" && o.Name == name {
			active = []swayOutput{o}
			break
		}
	}
	if len(active) != 1 {
		return 0, fmt.Errorf("cannot tell which of %d outputs the window is on", len(active))
	}
	if active[0].CurrentMode.Refresh <= 0 {
		return 0, fmt.Errorf("output %s reports no refresh rate", active[0].Name)
	}
	return 1e6 / float64(active[0].CurrentMode.Refresh), nil
}

// swayRequest sends one i3-ipc request to the sway socket and returns the
// payload of the answer.
func swayRequest(sock string, typ uint32) ([]byte, error) {
	c, err := net.DialTimeout("unix", sock, 2*time.Second)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	hdr := append([]byte("i3-ipc"), make([]byte, 8)...)
	binary.NativeEndian.PutUint32(hdr[6:], 0)
	binary.NativeEndian.PutUint32(hdr[10:], typ)
	if _, err := c.Write(hdr); err != nil {
		return nil, err
	}
	reply := make([]byte, 14)
	if _, err := io.ReadFull(c, reply); err != nil {
		return nil, err
	}
	if string(reply[:6]) != "i3-ipc" {
		return nil, errors.New("not an i3-ipc answer")
	}
	body := make([]byte, binary.NativeEndian.Uint32(reply[6:]))
	if _, err := io.ReadFull(c, body); err != nil {
		return nil, err
	}
	return body, nil
}

// swayInterval is the refresh interval of the output the app's window is
// on, asked of the sway the socket belongs to.
func swayInterval(sock string, pid int) (float64, error) {
	body, err := swayRequest(sock, 3) // GET_OUTPUTS
	if err != nil {
		return 0, err
	}
	var outputs []swayOutput
	if err := json.Unmarshal(body, &outputs); err != nil {
		return 0, err
	}
	name := ""
	if tree, err := swayRequest(sock, 4); err == nil { // GET_TREE
		var root swayNode
		if json.Unmarshal(tree, &root) == nil {
			name = windowOutput(root, pid, AppID)
		}
	}
	return outputInterval(outputs, name)
}
