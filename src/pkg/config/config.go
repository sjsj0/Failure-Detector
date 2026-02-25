package config

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

type Mode string

const (
	ModePingAck Mode = "pingack"
	ModeGossip  Mode = "gossip"
)

// Full, node-local config (bootstrap + cluster knobs).
type Config struct {
	// Cluster-wide (propagated)
	Version      int           `json:"version"`
	Mode         Mode          `json:"mode"`
	PingEvery    time.Duration `json:"ping_every"`
	PingFanout   int           `json:"ping_fanout"`
	GossipPeriod time.Duration `json:"gossip_period"`
	GossipFanout int           `json:"gossip_fanout"`
	TSuspect     time.Duration `json:"t_suspect"`
	TFail        time.Duration `json:"t_fail"`
	TCleanup     time.Duration `json:"t_cleanup"`
	DropRateRecv float64       `json:"drop_rate_recv"`

	// Local bootstrap (not propagated)
	SelfAddr           string `json:"self_addr"`                      // UDP "ip:port"
	BindAddr           string `json:"bind_addr"`                      // UDP bind "ip:port"
	IntroducerBindAddr string `json:"introducer_bind_addr,omitempty"` // introducer bind addr (optional, for NAT)
	Introducer         string `json:"introducer"`                     // UDP introducer (optional)
	IsIntroducer       bool   `json:"is_introducer"`                  // true if this node is the first
	AdminHTTP          string `json:"admin_http"`                     // ":8080" or "ip:port"
}

// Network-facing DTO. Only the cluster knobs go over the wire.
// Pointers let us detect "field not present" vs "present with zero value".
type ConfigDTO struct {
	Version      int     `json:"version,omitempty"`
	Mode         string  `json:"mode,omitempty"`
	PingEvery    string  `json:"ping_every,omitempty"`
	PingFanout   int     `json:"ping_fanout,omitempty"`
	GossipPeriod string  `json:"gossip_period,omitempty"`
	GossipFanout int     `json:"gossip_fanout,omitempty"`
	TSuspect     string  `json:"t_suspect,omitempty"`
	TFail        string  `json:"t_fail,omitempty"`
	TCleanup     string  `json:"t_cleanup,omitempty"`
	DropRateRecv float64 `json:"drop_rate_recv,omitempty"`
}

func Defaults() Config {
	hostname, _ := os.Hostname()
	return Config{
		// propagated
		Version: 1,
		Mode:    ModePingAck,
		//Mode:      ModeGossip,
		PingEvery: 200 * time.Millisecond,
		//AckTimeout:   250 * time.Millisecond,
		PingFanout:   1,
		GossipPeriod: 300 * time.Millisecond,
		GossipFanout: 3,
		TSuspect:     00000 * time.Millisecond,
		TFail:        2000 * time.Millisecond,
		TCleanup:     3000 * time.Millisecond,
		DropRateRecv: 0.0,

		// local bootstrap

		//get self address from the server hostname
		SelfAddr: hostname + ":5000", // TODO: derive this from OS hostname
		//SelfAddr:           "127.0.0.1:5000", // TODO: derive this from OS hostname
		BindAddr:           "0.0.0.0:5000",
		IntroducerBindAddr: "0.0.0.0:6000",
		Introducer:         "",
		IsIntroducer:       false,
		AdminHTTP:          ":8080",
	}
}

/* ------------------------- Centralized validation ------------------------- */

// ValidateLocalBootstrap: only local/identity bits.
func ValidateLocalBootstrap(c *Config) error {
	log.Printf("Validating local bootstrap: self=%q bind=%q introducer=%q is-introducer=%v admin=%q\n",
		c.SelfAddr, c.BindAddr, c.Introducer, c.IsIntroducer, c.AdminHTTP)
	// basic addr checks

	if err := validateUDPAddr(c.SelfAddr); err != nil {
		return fmt.Errorf("self addr: %w", err)
	}
	if err := validateUDPAddr(c.BindAddr); err != nil {
		return fmt.Errorf("bind addr: %w", err)
	}
	// exactly one of Introducer or IsIntroducer
	if (c.Introducer == "" && !c.IsIntroducer) || (c.Introducer != "" && c.IsIntroducer) {
		return errors.New("must specify either introducer OR is-introducer")
	}
	if c.Introducer != "" {
		if err := validateUDPAddr(c.Introducer); err != nil {
			return fmt.Errorf("introducer: %w", err)
		}
	}
	if err := validateAdminAddr(c.AdminHTTP); err != nil {
		return fmt.Errorf("admin http: %w", err)
	}
	return nil
}

// ValidateClusterParams normalizes/validates cluster-wide knobs (one place).
func ValidateClusterParams(c *Config) error {
	// Mode
	if c.Mode != ModePingAck && c.Mode != ModeGossip {
		return errors.New("invalid mode")
	}
	// Periods & counts
	if c.PingEvery <= 0 {
		return errors.New("ping_every must be > 0")
	}
	if c.PingFanout <= 0 {
		return errors.New("ping_fanout must be > 0")
	}
	if c.GossipPeriod <= 0 {
		return errors.New("gossip_period must be > 0")
	}
	if c.GossipFanout <= 0 {
		return errors.New("gossip_fanout must be > 0")
	}
	// Timers monotonic
	if c.TSuspect < 0 {
		return errors.New("t_suspect must be >= 0")
	}
	if c.TFail <= 0 {
		return errors.New("t_fail must be > 0")
	}
	if c.TCleanup <= 0 {
		return errors.New("t_cleanup must be > 0")
	}
	// Drop rate clamp
	if c.DropRateRecv < 0 || c.DropRateRecv > 1 {
		return errors.New("drop_rate_recv must be in [0,1]")
	}
	return nil
}

/* ------------------------ Flags (call once at boot) ----------------------- */

func LoadFromFlags() (Config, error) {
	def := Defaults()

	// bootstrap flags
	flag.StringVar(&def.BindAddr, "bind", def.BindAddr, "bind UDP addr (ip:port)")
	flag.StringVar(&def.SelfAddr, "self", def.SelfAddr, "self advertised UDP addr (ip:port)")
	flag.StringVar(&def.Introducer, "introducer", def.Introducer, "introducer UDP addr (optional)")
	flag.BoolVar(&def.IsIntroducer, "is-introducer", def.IsIntroducer, "this node is the introducer")
	flag.StringVar(&def.AdminHTTP, "admin", def.AdminHTTP, "admin http addr (:port or ip:port)")

	// cluster knobs (introducer or CLI changes)
	mode := flag.String("mode", string(def.Mode), "protocol mode: pingack|gossip")
	flag.DurationVar(&def.PingEvery, "ping-every", def.PingEvery, "ping period")
	flag.IntVar(&def.PingFanout, "ping-fanout", def.PingFanout, "ping fanout")
	flag.DurationVar(&def.GossipPeriod, "gossip-period", def.GossipPeriod, "gossip period")
	flag.IntVar(&def.GossipFanout, "gossip-fanout", def.GossipFanout, "gossip fanout")
	flag.DurationVar(&def.TSuspect, "t-suspect", def.TSuspect, "suspect after no alive for this")
	flag.DurationVar(&def.TFail, "t-fail", def.TFail, "fail after suspect/no updates")
	flag.DurationVar(&def.TCleanup, "t-cleanup", def.TCleanup, "delete after failed/left")
	flag.Float64Var(&def.DropRateRecv, "drop-recv", def.DropRateRecv, "receiver drop rate [0..1]")

	flag.Parse()

	// apply selected mode
	switch Mode(*mode) {
	case ModePingAck, ModeGossip:
		def.Mode = Mode(*mode)
	default:
		return Config{}, errors.New("invalid mode")
	}

	// centralized checks
	if err := ValidateLocalBootstrap(&def); err != nil {
		return Config{}, err
	}
	if err := ValidateClusterParams(&def); err != nil {
		return Config{}, err
	}
	return def, nil
}

/* -------------------- Config <-> DTO conversions/merge ------------------- */

// ToDTO builds a DTO with ALL cluster knobs (good default for gossip/forward).
func (c Config) ToDTO() ConfigDTO {
	return ConfigDTO{
		Version:      c.Version,
		Mode:         string(c.Mode),
		PingEvery:    c.PingEvery.String(),
		PingFanout:   c.PingFanout,
		GossipPeriod: c.GossipPeriod.String(),
		GossipFanout: c.GossipFanout,
		TSuspect:     c.TSuspect.String(),
		TFail:        c.TFail.String(),
		TCleanup:     c.TCleanup.String(),
		DropRateRecv: c.DropRateRecv,
	}
}

// ApplyDTO merges the provided DTO into c (in-place) with version checks.
// DTO is expected to be COMPLETE. Returns (changed, error).
func (c *Config) ApplyDTO(dto ConfigDTO) (bool, error) {
	next := *c

	// Version handling (must be strictly newer to apply)
	if dto.Version <= c.Version {
		return false, nil
	}
	next.Version = dto.Version

	// Mode (required)
	if dto.Mode == "" {
		return false, errors.New("mode missing")
	}
	m := Mode(dto.Mode)
	if m != ModePingAck && m != ModeGossip {
		return false, errors.New("invalid mode")
	}
	next.Mode = m

	// Durations (required; TSuspect may be "0s")
	var err error
	if dto.PingEvery == "" {
		return false, errors.New("ping_every missing")
	}
	if next.PingEvery, err = time.ParseDuration(dto.PingEvery); err != nil || next.PingEvery <= 0 {
		return false, errors.New("invalid ping_every")
	}

	if dto.GossipPeriod == "" {
		return false, errors.New("gossip_period missing")
	}
	if next.GossipPeriod, err = time.ParseDuration(dto.GossipPeriod); err != nil || next.GossipPeriod <= 0 {
		return false, errors.New("invalid gossip_period")
	}

	if dto.TSuspect == "" {
		return false, errors.New("t_suspect missing")
	}
	if next.TSuspect, err = time.ParseDuration(dto.TSuspect); err != nil || next.TSuspect < 0 {
		return false, errors.New("invalid t_suspect")
	}

	if dto.TFail == "" {
		return false, errors.New("t_fail missing")
	}
	if next.TFail, err = time.ParseDuration(dto.TFail); err != nil || next.TFail <= 0 {
		return false, errors.New("invalid t_fail")
	}

	if dto.TCleanup == "" {
		return false, errors.New("t_cleanup missing")
	}
	if next.TCleanup, err = time.ParseDuration(dto.TCleanup); err != nil || next.TCleanup <= 0 {
		return false, errors.New("invalid t_cleanup")
	}

	// Int knobs (required)
	if dto.PingFanout <= 0 {
		return false, errors.New("ping_fanout must be > 0")
	}
	next.PingFanout = dto.PingFanout

	if dto.GossipFanout <= 0 {
		return false, errors.New("gossip_fanout must be > 0")
	}
	next.GossipFanout = dto.GossipFanout

	// Float knob (required)
	if dto.DropRateRecv < 0 || dto.DropRateRecv > 1 {
		return false, errors.New("drop_rate_recv must be in [0,1]")
	}
	next.DropRateRecv = dto.DropRateRecv

	// Centralized normalization/validation for cluster knobs
	if err := ValidateClusterParams(&next); err != nil {
		return false, err
	}

	// Commit
	*c = next
	return true, nil
}

/* ------------------------------ helpers ---------------------------------- */

func validateUDPAddr(addr string) error {
	if addr == "" {
		return errors.New("empty")
	}
	if _, err := net.ResolveUDPAddr("udp", addr); err != nil {
		return err
	}
	return nil
}

// Accepts ":port" or "ip:port".
func validateAdminAddr(addr string) error {
	if addr == "" {
		return errors.New("empty")
	}
	if strings.HasPrefix(addr, ":") {
		p := strings.TrimPrefix(addr, ":")
		if _, err := strconv.Atoi(p); err != nil || p == "" {
			return errors.New("bad port")
		}
		return nil
	}
	_, err := net.ResolveTCPAddr("tcp", addr)
	return err
}

// ApplyRemote applies a DTO that must come from the network.
// Since DTO is value-based, Version is always present; ApplyDTO enforces Version > current.
func (c *Config) ApplyRemote(dto ConfigDTO) (bool, error) {
	return c.ApplyDTO(dto)
}

// BumpAndApplyLocal applies a DTO from local CLI/admin.
// Ensures the version is bumped locally (current+1) if not strictly higher.
func (c *Config) BumpAndApplyLocal(dto ConfigDTO) (bool, error) {
	if dto.Version <= c.Version {
		dto.Version = c.Version + 1
	}
	return c.ApplyDTO(dto)
}
