package execute

import (
	"github.com/coredhcp/coredhcp/handler"
	"github.com/coredhcp/coredhcp/logger"
	"github.com/coredhcp/coredhcp/plugins"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv6"
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

	/* This plugin is deprecated */

	return p.executeHandler6, nil
}

func setup4(args ...string) (handler.Handler4, error) {

	var (
		p   PluginState
	)
	/* This plugin is deprecated */

	return p.executeHandler4, nil
}
func (p *PluginState) executeHandler6(state *handler.PropagateState, req, resp dhcpv6.DHCPv6) (dhcpv6.DHCPv6, bool) {
	/* This plugin is deprecated */

	return resp, false
}

func (p *PluginState) executeHandler4(state *handler.PropagateState, req, resp *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, bool) {

	/* This plugin is deprecated */

	return resp, false
}
