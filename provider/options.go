package provider

import (
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/supabase/atlasctl/pkg/config"
)

// Option blocks mirror config.MsmOptions. A null block or attribute means
// "not set": the field is omitted from the API request. Any change to a block
// is a structural change in atlasctl (stop+create), so every block is
// RequiresReplace.

type commonOptionsModel struct {
	ResolveOnProbe      types.Bool    `tfsdk:"resolve_on_probe"`
	Spread              types.Int64   `tfsdk:"spread"`
	SkipDNSCheck        types.Bool    `tfsdk:"skip_dns_check"`
	TargetUpdateHours   types.Int64   `tfsdk:"target_update_hours"`
	AutoTopup           types.Bool    `tfsdk:"auto_topup"`
	AutoTopupPrbDaysOff types.Int64   `tfsdk:"auto_topup_prb_days_off"`
	AutoTopupPrbSimil   types.Float64 `tfsdk:"auto_topup_prb_similarity"`
	StartTime           types.String  `tfsdk:"start_time"`
	StopTime            types.String  `tfsdk:"stop_time"`
	BillTo              types.String  `tfsdk:"bill_to"`
}

type pingOptionsModel struct {
	Packets        types.Int64 `tfsdk:"packets"`
	PacketInterval types.Int64 `tfsdk:"packet_interval"`
	IncludeProbeID types.Bool  `tfsdk:"include_probe_id"`
}

type tracerouteOptionsModel struct {
	Protocol           types.String `tfsdk:"protocol"`
	ResponseTimeout    types.Int64  `tfsdk:"response_timeout"`
	Packets            types.Int64  `tfsdk:"packets"`
	Paris              types.Int64  `tfsdk:"paris"`
	FirstHop           types.Int64  `tfsdk:"first_hop"`
	MaxHops            types.Int64  `tfsdk:"max_hops"`
	DestinationOptSize types.Int64  `tfsdk:"destination_option_size"`
	HopByHopOptSize    types.Int64  `tfsdk:"hop_by_hop_option_size"`
	DontFragment       types.Bool   `tfsdk:"dont_fragment"`
}

type dnsOptionsModel struct {
	QueryType      types.String `tfsdk:"query_type"`
	QueryClass     types.String `tfsdk:"query_class"`
	Protocol       types.String `tfsdk:"protocol"`
	UseMacros      types.Bool   `tfsdk:"use_macros"`
	SetNSIDBit     types.Bool   `tfsdk:"set_nsid_bit"`
	IncludeQbuf    types.Bool   `tfsdk:"include_qbuf"`
	PrependProbeID types.Bool   `tfsdk:"prepend_probe_id"`
	SetRDBit       types.Bool   `tfsdk:"set_rd_bit"`
	SetDOBit       types.Bool   `tfsdk:"set_do_bit"`
	UDPPayloadSize types.Int64  `tfsdk:"udp_payload_size"`
	Retry          types.Int64  `tfsdk:"retry"`
	Timeout        types.Int64  `tfsdk:"timeout"`
}

type tlsOptionsModel struct {
	Port     types.Int64  `tfsdk:"port"`
	Hostname types.String `tfsdk:"hostname"`
}

type httpOptionsModel struct {
	QueryString        types.String `tfsdk:"query_string"`
	HeaderBytes        types.Int64  `tfsdk:"header_bytes"`
	ExtendedTiming     types.Bool   `tfsdk:"extended_timing"`
	MoreExtendedTiming types.Bool   `tfsdk:"more_extended_timing"`
}

func optString(desc string) schema.StringAttribute {
	return schema.StringAttribute{Optional: true, Description: desc}
}

func optInt(desc string) schema.Int64Attribute {
	return schema.Int64Attribute{Optional: true, Description: desc}
}

func optBool(desc string) schema.BoolAttribute {
	return schema.BoolAttribute{Optional: true, Description: desc}
}

func optionBlock(desc string, attrs map[string]schema.Attribute) schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional:    true,
		Description: desc,
		Attributes:  attrs,
		PlanModifiers: []planmodifier.Object{
			objectplanmodifier.RequiresReplace(),
		},
	}
}

// optionAttributes returns the schema for the six option blocks.
func optionAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"common": optionBlock("Options valid for every measurement type. Changing any value forces a new measurement.", map[string]schema.Attribute{
			"resolve_on_probe":          optBool("Resolve the target on each probe instead of once by RIPE Atlas."),
			"spread":                    optInt("Seconds over which probe start times are spread."),
			"skip_dns_check":            optBool("Skip the DNS check on the target when scheduling."),
			"target_update_hours":       optInt("Re-resolve the target every N hours."),
			"auto_topup":                optBool("Automatically replace probes that disconnect."),
			"auto_topup_prb_days_off":   optInt("Days a probe must be offline before auto topup replaces it."),
			"auto_topup_prb_similarity": schema.Float64Attribute{Optional: true, Description: "Similarity (0 to 1) required of auto topup replacement probes."},
			"start_time":                optString("Measurement start time, RFC 3339 (for example 2026-12-01T00:00:00Z)."),
			"stop_time":                 optString("Measurement stop time, RFC 3339. Must be after start_time."),
			"bill_to":                   optString("Account to bill credits to."),
		}),
		"ping": optionBlock("Ping options. Only valid when msm_type is ping.", map[string]schema.Attribute{
			"packets":          optInt("Packets per probe per run (API default 3)."),
			"packet_interval":  optInt("Milliseconds between packets."),
			"include_probe_id": optBool("Include the probe ID in the packet payload."),
		}),
		"traceroute": optionBlock("Traceroute options. Only valid when msm_type is traceroute.", map[string]schema.Attribute{
			"protocol":                optString("ICMP, UDP (atlasctl default), or TCP."),
			"response_timeout":        optInt("Response timeout in milliseconds."),
			"packets":                 optInt("Packets per hop."),
			"paris":                   optInt("Paris variations."),
			"first_hop":               optInt("First TTL."),
			"max_hops":                optInt("Maximum number of hops."),
			"destination_option_size": optInt("Destination options header size."),
			"hop_by_hop_option_size":  optInt("Hop-by-hop options header size."),
			"dont_fragment":           optBool("Set the don't fragment bit."),
		}),
		"dns": optionBlock("DNS options. Only valid when msm_type is dns.", map[string]schema.Attribute{
			"query_type":       optString("Record type, for example A (default), AAAA, NS, MX."),
			"query_class":      optString("IN (default) or CHAOS."),
			"protocol":         optString("UDP (default) or TCP."),
			"use_macros":       optBool("Enable macro expansion in the query."),
			"set_nsid_bit":     optBool("Set the NSID bit."),
			"include_qbuf":     optBool("Include the query buffer in results."),
			"prepend_probe_id": optBool("Prepend the probe ID to the query."),
			"set_rd_bit":       optBool("Set the recursion desired bit."),
			"set_do_bit":       optBool("Set the DNSSEC OK bit."),
			"udp_payload_size": optInt("EDNS0 UDP payload size."),
			"retry":            optInt("Number of retries."),
			"timeout":          optInt("Timeout in milliseconds."),
		}),
		"tls": optionBlock("TLS options. Only valid when msm_type is tls.", map[string]schema.Attribute{
			"port":     optInt("TCP port (API default 443)."),
			"hostname": optString("Server name for SNI."),
		}),
		"http": optionBlock("Additional HTTP options. Only valid when msm_type is http.", map[string]schema.Attribute{
			"query_string":         optString("Query string appended to the path."),
			"header_bytes":         optInt("Number of header bytes to return."),
			"extended_timing":      optBool("Include extended timing results."),
			"more_extended_timing": optBool("Include more extended timing results."),
		}),
	}
}

func str(v types.String) string { return v.ValueString() }
func u(v types.Int64) uint      { return uint(v.ValueInt64()) }

// buildMsmOptions converts the option blocks of m into config.MsmOptions.
// Null values map to the zero value, which atlasctl treats as "not set".
func buildMsmOptions(m measurementModel) config.MsmOptions {
	var o config.MsmOptions
	if c := m.Common; c != nil {
		o.Common = config.CommonOptions{
			ResolveOnProbe:      c.ResolveOnProbe.ValueBool(),
			Spread:              u(c.Spread),
			SkipDNSCheck:        c.SkipDNSCheck.ValueBool(),
			TargetUpdateHours:   u(c.TargetUpdateHours),
			AutoTopup:           c.AutoTopup.ValueBool(),
			AutoTopupDaysOff:    u(c.AutoTopupPrbDaysOff),
			AutoTopupSimilarity: c.AutoTopupPrbSimil.ValueFloat64(),
			StartTime:           str(c.StartTime),
			StopTime:            str(c.StopTime),
			BillTo:              str(c.BillTo),
		}
	}
	if p := m.Ping; p != nil {
		o.Ping = config.PingOptions{
			Packets:        u(p.Packets),
			PacketInterval: u(p.PacketInterval),
			IncludeProbeID: p.IncludeProbeID.ValueBool(),
		}
	}
	if t := m.Traceroute; t != nil {
		o.Traceroute = config.TraceOptions{
			Protocol:           str(t.Protocol),
			ResponseTimeout:    u(t.ResponseTimeout),
			Packets:            u(t.Packets),
			Paris:              u(t.Paris),
			FirstHop:           u(t.FirstHop),
			MaxHops:            u(t.MaxHops),
			DestinationOptSize: u(t.DestinationOptSize),
			HopByHopOptSize:    u(t.HopByHopOptSize),
			DontFragment:       t.DontFragment.ValueBool(),
		}
	}
	if d := m.DNS; d != nil {
		o.DNS = config.DNSOptions{
			QueryType:      str(d.QueryType),
			QueryClass:     str(d.QueryClass),
			Protocol:       str(d.Protocol),
			UseMacros:      d.UseMacros.ValueBool(),
			SetNSIDBit:     d.SetNSIDBit.ValueBool(),
			IncludeQbuf:    d.IncludeQbuf.ValueBool(),
			PrependProbeID: d.PrependProbeID.ValueBool(),
			SetRDBit:       d.SetRDBit.ValueBool(),
			SetDOBit:       d.SetDOBit.ValueBool(),
			UDPPayloadSize: u(d.UDPPayloadSize),
			Retry:          u(d.Retry),
			Timeout:        u(d.Timeout),
		}
	}
	if t := m.TLS; t != nil {
		o.TLS = config.TLSOptions{Port: u(t.Port), Hostname: str(t.Hostname)}
	}
	if h := m.HTTP; h != nil {
		o.HTTP = config.HTTPOptions{
			QueryString:        str(h.QueryString),
			HeaderBytes:        u(h.HeaderBytes),
			ExtendedTiming:     h.ExtendedTiming.ValueBool(),
			MoreExtendedTiming: h.MoreExtendedTiming.ValueBool(),
		}
	}
	return o
}

var (
	validTraceProtocols = map[string]bool{"ICMP": true, "UDP": true, "TCP": true}
	validDNSProtocols   = map[string]bool{"UDP": true, "TCP": true}
	validDNSClasses     = map[string]bool{"IN": true, "CHAOS": true}
)

// validateOptions rejects option blocks that do not match msm_type and checks
// the enum and range values that atlasctl would otherwise only reject at apply.
func validateOptions(m measurementModel) diag.Diagnostics {
	var diags diag.Diagnostics

	for _, b := range []struct {
		name string
		set  bool
		want string
	}{
		{"ping", m.Ping != nil, "ping"},
		{"traceroute", m.Traceroute != nil, "traceroute"},
		{"dns", m.DNS != nil, "dns"},
		{"tls", m.TLS != nil, "tls"},
		{"http", m.HTTP != nil, "http"},
	} {
		if b.set && !m.MsmType.IsUnknown() && m.MsmType.ValueString() != b.want {
			diags.AddAttributeError(path.Root(b.name), fmt.Sprintf("%s block requires %s type", b.name, b.want),
				fmt.Sprintf("The %s block is only valid when msm_type is %q.", b.name, b.want))
		}
	}

	enum := func(block, attr string, v types.String, allowed map[string]bool, hint string) {
		if s := v.ValueString(); s != "" && !allowed[s] {
			diags.AddAttributeError(path.Root(block).AtName(attr), fmt.Sprintf("Invalid %s.%s", block, attr),
				fmt.Sprintf("%s.%s must be %s. Got %q.", block, attr, hint, s))
		}
	}
	if t := m.Traceroute; t != nil {
		enum("traceroute", "protocol", t.Protocol, validTraceProtocols, "ICMP, UDP, or TCP")
	}
	if d := m.DNS; d != nil {
		enum("dns", "protocol", d.Protocol, validDNSProtocols, "UDP or TCP")
		enum("dns", "query_class", d.QueryClass, validDNSClasses, "IN or CHAOS")
	}

	if c := m.Common; c != nil {
		if !c.AutoTopupPrbSimil.IsNull() && !c.AutoTopupPrbSimil.IsUnknown() {
			if s := c.AutoTopupPrbSimil.ValueFloat64(); s < 0 || s > 1 {
				diags.AddAttributeError(path.Root("common").AtName("auto_topup_prb_similarity"),
					"Invalid common.auto_topup_prb_similarity",
					fmt.Sprintf("auto_topup_prb_similarity must be between 0 and 1. Got %v.", s))
			}
		}
		var start, stop time.Time
		for _, f := range []struct {
			attr string
			val  types.String
			out  *time.Time
		}{{"start_time", c.StartTime, &start}, {"stop_time", c.StopTime, &stop}} {
			if f.val.IsNull() || f.val.IsUnknown() {
				continue
			}
			t, err := time.Parse(time.RFC3339, f.val.ValueString())
			if err != nil {
				diags.AddAttributeError(path.Root("common").AtName(f.attr), "Invalid "+f.attr,
					fmt.Sprintf("%s must be RFC 3339 (for example 2026-12-01T00:00:00Z). Got %q.", f.attr, f.val.ValueString()))
				continue
			}
			*f.out = t
		}
		if !start.IsZero() && !stop.IsZero() && !stop.After(start) {
			diags.AddAttributeError(path.Root("common").AtName("stop_time"), "Invalid stop_time",
				"stop_time must be after start_time.")
		}
	}

	return diags
}
