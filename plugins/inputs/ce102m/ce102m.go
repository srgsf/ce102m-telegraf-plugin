package ce102m

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/config"
	"github.com/influxdata/telegraf/plugins/inputs"
	"github.com/srgsf/ce102m-telegraf-plugin/log"
	"github.com/srgsf/iec62056.golang"
)

const maxTariffId = 5
const maxRetries = 3
const (
	keyMeterId   = "id"
	keyError     = "error_key"
	keyErrDesc   = "error_description"
	keyNetStatus = "net_status"
	measurment   = "ce102m"
)

var version = "dev"

// ce102m,id net_status,error_cd,error_desc,chan_X time
type device struct {
	Socket          string
	Address         string
	Status          Duration      `toml:"status_interval"`
	Timzone         string        `toml:"systime_tz"`
	LogProto        bool          `toml:"log_protocol"`
	Pass            []int         `toml:"tariff_include"`
	Prefix          string        `toml:"tariff_prefix"`
	FixParity       bool          `toml:"software_parity"`
	LogLevel        *log.LogLevel `toml:"log_level"`
	tariffs         *tariffFilter
	cl              client
	location        *time.Location
	meterId         string
	lastStatusTime  time.Time
	prevIsConnected bool
}

type client struct {
	dialer      iec62056.TCPDialer
	td          *iec62056.TariffDevice
	conn        iec62056.Conn
	isConnected bool
	idleTo      time.Duration
}

type result struct {
	tags   map[string]string
	fields map[string]any
}

type tariffFilter struct {
	isAll    bool
	isSingle bool
	mask     [maxTariffId]bool
	minId    int
	arg      string
}

func newTariffFilter(cfg []int) (*tariffFilter, error) {
	rv := &tariffFilter{}
	switch len(cfg) {
	case 0:
		rv.isAll = true
		return rv, nil
	case 1:
		arg := cfg[0]
		if 0 >= arg || arg > maxTariffId {
			return nil, fmt.Errorf("invalid tariff %d", arg)
		}
		rv.arg = fmt.Sprint(arg)
		rv.isSingle = true
		rv.minId = arg
		rv.mask[arg-1] = true
		return rv, nil
	}
	sort.Ints(cfg)

	for _, t := range cfg {
		if 0 >= t || t > maxTariffId {
			return nil, fmt.Errorf("invalid tariff %d", t)
		}
		rv.mask[t-1] = true
	}
	rv.minId = cfg[0]
	maxId := cfg[len(cfg)-1]
	if rv.minId == maxId {
		rv.arg = fmt.Sprint(rv.minId)
		rv.isSingle = true
		return rv, nil
	}

	all := true
	for _, v := range rv.mask {
		if !v {
			all = false
			break
		}
	}

	if all {
		rv.isAll = true
	}
	rv.arg = fmt.Sprintf("%d,%d", rv.minId, maxId-rv.minId+1)
	return rv, nil
}

func (d *device) Init() error {
	logLevel := log.LvlInfo
	if d.LogLevel != nil {
		logLevel = *d.LogLevel
	}

	log.InitLoggers(os.Stderr, logLevel)

	log.Infof("ce102m plugin version: %s", version)
	log.Debug("plugin initialization started")
	if d.Socket == "" {
		log.Error("socket configuration property is not set")
		return errors.New("socket is required")
	}

	d.location = time.UTC
	if d.Timzone != "" {
		location, err := time.LoadLocation(d.Timzone)
		if err != nil {
			log.Error("invalid systime_tz value")
			return errors.New("invalid timezone")
		}
		d.location = location
	}

	if d.LogProto {
		log.Debug("protocol logger enabled")
		d.cl.dialer.ProtocolLogger = log.DEBUG
	}

	d.cl.dialer.SwParity = d.FixParity
	d.cl.dialer.ConnectionTimeOut = 20 * time.Second
	d.cl.dialer.RWTimeOut = 10 * time.Second
	tf, err := newTariffFilter(d.Pass)
	if err != nil {
		log.Error("unable to configure tariffs")
		return err
	}
	d.tariffs = tf
	log.Debug("plugin initialization finished")
	return nil
}

func (d *device) SampleConfig() string {
	return `
## Gather data from ce102m power meter ##
[[inputs.ce102m]]
    ## tcp socket address for rs485 to ethernet converter.
    socket ="localhost:4001"
    ## device address - optional for broadcast.
    # address = ""
    ## If even parity should be handled manually.
    software_parity = true
    ## Status request interval - don't request if ommited or 0
    status_interval = "1d"
    ## Timezone of device system time.
    systime_tz = "Europe/Moscow"
    ## should protocol be logged as debug output.
    # log_protocol = true
    ## log level. Possible values are error,warning,info,debug
    #log_level = "info"
    ## query only the following tariffs starts with 1 for summary.
    tariff_include = [2,3]
    ## value prefix for a tariff
    tariff_prefix = "chan_"
`
}

func (d *device) Description() string {
	return "Reads ce102m power meter data via tcp"
}

func (d *device) Gather(acc telegraf.Accumulator) error {
	t, err := d.gatherData(acc)
	if d.prevIsConnected != d.cl.isConnected {
		status := "offline"
		if d.cl.isConnected {
			status = "online"
		}
		acc.AddFields(measurment, map[string]any{keyNetStatus: status},
			map[string]string{keyMeterId: d.meterId}, t)
		d.prevIsConnected = d.cl.isConnected
	}
	return err
}

func (d *device) gatherData(acc telegraf.Accumulator) (time.Time, error) {
	withRetries := func(fn func() error) error {
		var err error
		for i := 0; i < maxRetries; i++ {
			if err = fn(); err == nil {
				break
			}
		}
		return err
	}

	var t time.Time
	err := withRetries(func() error {
		var err error
		t, err = d.systime()
		return err
	})

	if err != nil {
		log.Warnf("ce102m systime gather error: %s", err.Error())
		return time.Now(), err
	}

	err = withRetries(func() error {
		stat, err := d.gatherStatus()
		if err == nil {
			for _, v := range stat {
				acc.AddFields(measurment, v.fields, v.tags, t)
			}
		}
		return err
	})

	if err != nil {
		log.Warnf("ce102m status gather error: %s", err.Error())
		return t, err
	}

	err = withRetries(func() error {
		v, err := d.gatherValues()
		if err == nil {
			acc.AddFields(measurment, v.fields, v.tags, t)
		}
		return err
	})

	if err != nil {
		log.Warnf("ce102m values gather error: %s", err.Error())
	}
	return t, err
}

func (d *device) gatherStatus() ([]result, error) {
	if d.Status.Empty() || d.Status.Until(d.lastStatusTime) >= 0 {
		return nil, nil
	}

	stat, err := d.curState()
	if err != nil {
		return nil, err
	}
	var rv []result

	for k, v := range stat {
		rv = append(rv, result{
			tags:   map[string]string{keyMeterId: d.meterId},
			fields: map[string]any{keyError: k, keyErrDesc: v},
		})
	}
	d.lastStatusTime = time.Now()
	return rv, nil
}

func (d *device) gatherValues() (*result, error) {
	cmd := iec62056.Command{
		Id:      iec62056.CmdR1,
		Payload: &iec62056.DataSet{Address: "ET0PE"},
	}

	if !d.tariffs.isAll {
		cmd.Payload.Value = d.tariffs.arg
	}

	log.Debug("current values request started")
	db, err := d.commandToMeter(cmd)
	if err != nil {
		log.Warn("current values request failed")
		return nil, err
	}
	if len(db.Lines) == 0 || len(db.Lines[0].Sets) == 0 {
		log.Warn("current values empty result")
		return nil, errors.New("empty result for current values request")
	}

	log.Debug("current values received")
	rv := &result{
		tags:   map[string]string{keyMeterId: d.meterId},
		fields: map[string]any{},
	}

	parseValue := func(v string) (uint64, error) {
		v = strings.Replace(v, ".", "", 1)
		return strconv.ParseUint(v, 10, 32)
	}

	for i, dl := range db.Lines {
		if maxTariffId < i+1 {
			break
		}
		for _, ds := range dl.Sets {
			if d.tariffs.isSingle {
				val, err := parseValue(ds.Value)
				if err != nil {
					log.Warnf("unable to parse value %s", ds.Value)
					return nil, err
				}
				rv.fields[fmt.Sprintf("%s%d", d.Prefix, d.tariffs.minId)] = val
				return rv, nil
			}
			if d.tariffs.isAll || d.tariffs.mask[d.tariffs.minId+i-1] {
				val, err := parseValue(ds.Value)
				if err != nil {
					log.Warnf("unable to parse value %s", ds.Value)
					return nil, err
				}
				tid := 1
				if !d.tariffs.isAll {
					tid = d.tariffs.minId
				}
				rv.fields[fmt.Sprintf("%s%d", d.Prefix, i+tid)] = val
			}
		}
	}
	log.Debug("current values request succeed")
	return rv, nil
}

func (d *device) client() (*iec62056.TariffDevice, error) {
	if d.cl.isConnected {
		return d.cl.td, nil
	}
	if d.cl.conn != nil {
		_ = d.cl.conn.Close()
		d.cl.conn = nil
	}
	log.Debug("connection to meter started")
	conn, err := d.cl.dialer.Dial(d.Socket)
	if err != nil {
		log.Warn("connection to meter failed")
		return nil, err
	}
	log.Debug("connection to meter succeed")
	if d.cl.td == nil {
		d.cl.td = iec62056.WithAddress(conn, d.Address)
	}
	d.cl.td.Reset(conn)
	if d.cl.idleTo == 0 {
		log.Debug("session timeout request started")
		to, err := getIdleTimeout(d.cl.td)
		if err != nil {
			log.Warn("session timeout request failed")
			_ = conn.Close()
			return nil, err
		}
		d.cl.idleTo = to
		log.Debug("session timeout request succeed")
	}
	d.cl.td.IdleTimeout = d.cl.idleTo
	if d.meterId == "" {
		log.Debug("meter id request started")
		id, err := getMeterId(d.cl.td)
		if err != nil {
			log.Warn("meter id request failed")
			_ = conn.Close()
			return nil, err
		}
		d.meterId = id
		log.Debug("meter id request succeed")
	}
	d.cl.conn = conn
	d.cl.isConnected = true
	return d.cl.td, nil
}

func (d *device) commandToMeter(cmd iec62056.Command) (*iec62056.DataBlock, error) {
	td, err := d.client()
	if err != nil {
		log.Warn("unable to connect")
		return nil, err
	}
	db, err := td.Command(cmd)
	if err != nil {
		d.cl.isConnected = false
	}
	return db, err
}

func (d *device) systime() (time.Time, error) {
	log.Debug("device systime request started")

	var cmd = iec62056.Command{
		Id:      iec62056.CmdR1,
		Payload: &iec62056.DataSet{Address: "DATE_"},
	}
	log.Debug("device date request started")
	db, err := d.commandToMeter(cmd)
	if err != nil {
		log.Warn("device date request failed")
		return time.Time{}, err
	}

	if len(db.Lines) == 0 || len(db.Lines[0].Sets) == 0 {
		log.Warn("device date empty result")
		return time.Time{}, errors.New("empty result for system date request")
	}
	var sb strings.Builder
	dateStr := db.Lines[0].Sets[0].Value
	dateStr = dateStr[3:]
	sb.WriteString(dateStr)
	sb.WriteRune(' ')
	log.Debug("device date request succeed")

	cmd.Payload = &iec62056.DataSet{Address: "TIME_"}
	log.Debug("device time request started")
	db, err = d.commandToMeter(cmd)
	if err != nil {
		log.Warn("device time request failed")
		return time.Time{}, err
	}

	if len(db.Lines) == 0 || len(db.Lines[0].Sets) == 0 {
		log.Warn("device time empty result")
		return time.Time{}, errors.New("empty result for system time request")
	}
	sb.WriteString(db.Lines[0].Sets[0].Value)
	log.Debug("device time request succeed")
	tm, err := time.ParseInLocation("02.01.06 15:04:05", sb.String(), d.location)
	if err != nil {
		log.Warn("device systime parse failed")
		return time.Time{}, fmt.Errorf("unable to parse date string: '%s'", sb.String())
	}
	log.Debug("device systime request succeed")
	return tm, nil
}

func (d *device) curState() (map[string]string, error) {
	var cmd = iec62056.Command{
		Id:      iec62056.CmdR1,
		Payload: &iec62056.DataSet{Address: "STAT_"},
	}

	log.Debug("device status request started")
	db, err := d.commandToMeter(cmd)
	if err != nil {
		log.Warn("device status request failed")
		return nil, err
	}
	if len(db.Lines) == 0 || len(db.Lines[0].Sets) == 0 {
		log.Warn("device status empty result")
		return nil, errors.New("empty result for system status request")
	}
	val, err := strconv.ParseInt(db.Lines[0].Sets[0].Value, 16, 32)
	if err != nil {
		log.Warn("device status parse error")
		return nil, errors.New("system status parse error")
	}
	log.Debug("device status request succeed")
	rv := make(map[string]string)

	if val&(0x1<<3) != 0 {
		rv["BatDischarged"] = "Battery discharged"
	}
	if val&(0x1<<12) != 0 {
		rv["TimeSync"] = "Time is not syncronizeded"
	}
	if val&(0x1<<16) != 0 {
		rv["PowChecksum"] = "Checksum of power parameters mismatch"
	}
	if val&(0x1<<17) != 0 {
		rv["IllegalAccess"] = "Illegal access detected"
	}
	if val&(0x1<<19) != 0 {
		rv["BatExpired"] = "Battery expired"
	}
	if val&(0x1<<20) != 0 {
		rv["EEPROM"] = "EEPROM checksum mismatch"
	}
	if val&(0x1<<21) != 0 {
		rv["DeviceParam"] = "Checksum of device parameters mismatch"
	}
	if val&(0x1<<28) != 0 {
		rv["Scheduler"] = "Scheduler configuration has errors"
	}
	return rv, nil
}

func getIdleTimeout(td *iec62056.TariffDevice) (time.Duration, error) {
	db, err := td.Command(iec62056.Command{
		Id:      iec62056.CmdR1,
		Payload: &iec62056.DataSet{Address: "ACTIV"},
	})
	if err != nil {
		return 0, err
	}
	if len(db.Lines) == 0 || len(db.Lines[0].Sets) == 0 {
		return 0, errors.New("session imeout returned empty result")
	}
	txt := db.Lines[0].Sets[0].Value
	v, err := strconv.Atoi(txt)
	if err == nil {
		return time.Duration(v) * time.Second, nil
	}
	return 0, err
}

func getMeterId(td *iec62056.TariffDevice) (string, error) {
	db, err := td.Command(iec62056.Command{
		Id:      iec62056.CmdR1,
		Payload: &iec62056.DataSet{Address: "SNUMB"},
	})
	if err != nil {
		return "", err
	}
	if len(db.Lines) == 0 || len(db.Lines[0].Sets) == 0 {
		err = errors.New("empty result for serial no request")
		return "", err
	}
	return db.Lines[0].Sets[0].Value, nil
}

func init() {
	inputs.Add("ce102m", func() telegraf.Input {
		return &device{}
	})
}

type Duration struct {
	tt    time.Duration
	monts int
	years int
}

// UnmarshalTOML parses the duration from the TOML config file
func (d *Duration) UnmarshalTOML(b []byte) error {
	var cd config.Duration
	err := cd.UnmarshalTOML(b)
	if err == nil {
		*d = Duration{tt: time.Duration(cd)}
		return nil
	}

	isDigit := func(ch rune) bool { return ('0' <= ch && ch <= '9') }
	newErr := func() error {
		return fmt.Errorf("invalid durtion: %s", string(b))
	}

	dR := []rune(string(b))
	*d = Duration{}
	var i int
	for i < len(dR) {
		s := i
		for ; i < len(dR) && isDigit(dR[i]); i++ {
			//digits
		}
		if i >= len(dR) || i == s {
			return newErr()
		}
		n, err := strconv.ParseInt(string(dR[s:i]), 10, 64)
		if err != nil {
			return newErr()
		}
		switch dR[i] {
		case 's':
			d.tt += time.Duration(n) * time.Second
		case 'h':
			d.tt += time.Duration(n) * time.Hour
		case 'd':
			d.tt += time.Duration(n) * 24 * time.Hour
		case 'w':
			d.tt += time.Duration(n) * 7 * 24 * time.Hour
		case 'y':
			d.years = int(n)
		case 'n':
			d.tt += time.Duration(n) * time.Nanosecond
			if i+1 < len(dR) && dR[i+1] == 's' {
				i += 2
				continue
			}
		case 'u', 'µ':
			d.tt += time.Duration(n) * time.Microsecond
			if i+1 < len(dR) && dR[i+1] == 's' {
				i += 2
				continue
			}
		case 'm':
			if i+1 < len(dR) && dR[i+1] == 's' {
				d.tt += time.Duration(n) * time.Millisecond
				i += 2
				continue
			}

			if i+1 < len(dR) && dR[i+1] == 'o' {
				d.monts = int(n)
				i += 2
				continue
			}
			d.tt += time.Duration(n) * time.Minute
		default:
			return newErr()
		}
		i++
	}
	return nil
}

func (d *Duration) UnmarshalText(text []byte) error {
	return d.UnmarshalTOML(text)
}

func (d *Duration) Empty() bool {
	return d.tt == 0 && d.years == 0 && d.monts == 0
}
func (d *Duration) Until(t time.Time) time.Duration {
	t = t.AddDate(d.years, d.monts, 0)
	t = t.Add(d.tt)
	return time.Until(t)
}
