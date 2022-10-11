package execute

import (
	"github.com/coredhcp/coredhcp/handler"
	"github.com/coredhcp/coredhcp/logger"
	"github.com/coredhcp/coredhcp/plugins"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv6"
	"fmt"
	"regexp"
	"os/exec"
)

var log = logger.GetLogger("plugins/execute")

var Plugin = plugins.Plugin{
	Name:   "execute",
	Setup6: setup6,
	Setup4: setup4,
}

type PluginState struct {
	Script string
}
func setup6(args ...string) (handler.Handler6, error) {

	var (
		p   PluginState
	)

	if len(args) != 1 {
		return nil, fmt.Errorf("invalid number of arguments, want: 1 (script file), got: %d", len(args))
	}

	p.Script = args[0]

	log.Printf("loaded plugin for DHCPv6.")
	return p.executeHandler6, nil
}

func setup4(args ...string) (handler.Handler4, error) {

	var (
		p   PluginState
	)

	if len(args) != 1 {
		return nil, fmt.Errorf("invalid number of arguments, want: 1 (script file), got: %d", len(args))
	}

	log.Printf("loaded plugin for DHCPv4.")


	p.Script = args[0]

	return p.executeHandler4, nil
}
func (p *PluginState) executeHandler6(state *handler.PropagateState, req, resp dhcpv6.DHCPv6) (dhcpv6.DHCPv6, bool) {
	log.Printf("received DHCPv6 packet: %s", req.Summary())
	log.Printf("Made response: %s", resp.Summary())

	log.Fatalf("DHCPv6 not implemented")


	return resp, false
}

func (p *PluginState) executeHandler4(state *handler.PropagateState, req, resp *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, bool) {
	reg, err := regexp.Compile("[^A-Za-z0-9.-_]+")
	if err != nil {
		log.Fatalf("regexp failed: %v", err)
	}
	filteredHostName := reg.ReplaceAllString(req.HostName(), "")
        if filteredHostName == "" {
          filteredHostName = "DefaultMissingName"
        }

	interfaceName := string(state.InterfaceName)
	interfaceName = reg.ReplaceAllString(interfaceName, "")

	cmd := exec.Command(p.Script, resp.YourIPAddr.String(), resp.ClientHWAddr.String(), filteredHostName, interfaceName, resp.Router()[0].String())

	err = cmd.Run()
	if err != nil {
		log.Infof("cmd.Run() failed with %s\n", err)
	}

	return resp, false
}
