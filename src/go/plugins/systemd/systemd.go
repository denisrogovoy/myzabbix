package systemd

import (
	"encoding/json"
	"fmt"
	"go/internal/plugin"
	"path/filepath"
	"sync"

	"github.com/godbus/dbus"
)

// Plugin -
type Plugin struct {
	plugin.Base
	conn  []*dbus.Conn
	mutex sync.Mutex
}

var impl Plugin

type unit struct {
	Name        string `json:"{#UNIT.NAME}"`
	Description string `json:"{#UNIT.DESCRIPTION}"`
	LoadState   string `json:"{#UNIT.LOADSTATE}"`
	ActiveState string `json:"{#UNIT.ACTIVESTATE}"`
	SubState    string `json:"{#UNIT.SUBSTATE}"`
	Followed    string `json:"{#UNIT.FOLLOWED}"`
	Path        string `json:"{#UNIT.PATH}"`
	JobID       uint32 `json:"{#UNIT.JOBID}"`
	JobType     string `json:"{#UNIT.JOBTYPE}"`
	JobPath     string `json:"{#UNIT.JOBPATH}"`
}

func getConnection() (*dbus.Conn, error) {
	var err error
	var conn *dbus.Conn

	impl.mutex.Lock()
	defer impl.mutex.Unlock()

	if len(impl.conn) == 0 {
		conn, err = dbus.SystemBusPrivate()
		if err != nil {
			return nil, err
		}
		err = conn.Auth(nil)
		if err != nil {
			conn.Close()
			return nil, err
		}

		err = conn.Hello()
		if err != nil {
			conn.Close()
			return nil, err
		}

	} else {
		conn = impl.conn[len(impl.conn)-1]
		impl.conn = impl.conn[:len(impl.conn)-1]
	}

	return conn, nil
}

func releaseConnection(conn *dbus.Conn) {
	impl.mutex.Lock()
	defer impl.mutex.Unlock()
	impl.conn = append(impl.conn, conn)
}

func zbxNum2hex(c byte) byte {
	if c >= 10 {
		return c + 0x57 /* a-f */
	}
	return c + 0x30 /* 0-9 */
}

// Export -
func (p *Plugin) Export(key string, params []string) (interface{}, error) {
	conn, err := getConnection()

	if nil != err {
		return nil, fmt.Errorf("Cannot establish connection to any available bus: %s", err)
	}

	defer releaseConnection(conn)

	switch key {
	case "systemd.unit.discovery":
		var ext string

		if len(params) > 1 {
			return nil, fmt.Errorf("Too many parameters.")
		}

		if len(params) == 0 || len(params[0]) == 0 {
			ext = ".service"
		} else {
			switch params[0] {
			case "service", "target", "automount", "device", "mount", "path", "scope", "slice", "snapshot", "socket", "swap", "timer":
				ext = "." + params[0]
			case "all":
				ext = ""
			default:
				return nil, fmt.Errorf("Invalid first parameter.")
			}
		}

		var units []unit
		obj := conn.Object("org.freedesktop.systemd1", dbus.ObjectPath("/org/freedesktop/systemd1"))
		err := obj.Call("org.freedesktop.systemd1.Manager.ListUnits", 0).Store(&units)

		if nil != err {
			return nil, fmt.Errorf("Cannot retrieve list of units: %s", err)
		}

		array := make([]*unit, len(units))
		j := 0
		for i := 0; i < len(units); i++ {
			if len(ext) != 0 && ext != filepath.Ext(units[i].Name) {
				continue
			}
			array[j] = &units[i]
			j++
		}

		jsonArray, err := json.Marshal(array[:j])

		if nil != err {
			return nil, fmt.Errorf("Cannot create JSON array: %s", err)
		}

		return string(jsonArray), nil
	case "systemd.unit.info":
		var property, unitType string
		var value interface{}

		if len(params) > 3 {
			return nil, fmt.Errorf("Too many parameters.")
		}

		if len(params) < 1 || len(params[0]) == 0 {
			return nil, fmt.Errorf("Invalid first parameter.")
		}

		if len(params) < 2 || len(params[1]) == 0 {
			property = "ActiveState"
		} else {
			property = params[1]
		}

		if len(params) < 3 || len(params[2]) == 0 || params[2] == "Unit" {
			unitType = "Unit"
		} else {
			unitType = params[2]
		}

		name := params[0]

		nameEsc := make([]byte, len(name)*3)
		j := 0
		for i := 0; i < len(name); i++ {
			if (name[i] >= 'A' && name[i] <= 'Z') ||
				(name[i] >= 'a' && name[i] <= 'z') ||
				((name[i] >= '0' && name[i] <= '9') && i != 0) {
				nameEsc[j] = name[i]
				j++
				continue
			}
			nameEsc[j] = '_'
			j++
			nameEsc[j] = zbxNum2hex((name[i] >> 4) & 0xf)
			j++
			nameEsc[j] = zbxNum2hex(name[i] & 0xf)
			j++
		}

		obj := conn.Object("org.freedesktop.systemd1", dbus.ObjectPath("/org/freedesktop/systemd1/unit/"+string(nameEsc[:j])))
		err := obj.Call("org.freedesktop.DBus.Properties.Get", 0, "org.freedesktop.systemd1."+unitType, property).Store(&value)

		if nil != err {
			return nil, fmt.Errorf("Cannot get unit property: %s", err)
		}

		return value, nil
	}

	return nil, fmt.Errorf("Not implemented: %s", key)
}

func init() {
	plugin.RegisterMetric(&impl, "systemd", "systemd.unit.discovery", "Returns JSON array of discovered units, usage: systemd.unit.discovery[<type>]")
	plugin.RegisterMetric(&impl, "systemd", "systemd.unit.info", "Returns the unit info, usage: systemd.unit.info[unit,<parameter>,<interface>]")
}
