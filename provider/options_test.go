package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestValidateOptions_WrongTypeBlock(t *testing.T) {
	m := validDNSModel(t)
	m.Ping = &pingOptionsModel{Packets: int64Val(5)}
	if !hasError(validateMeasurement(m), "ping block requires ping type") {
		t.Error("expected wrong-type error for ping block on dns measurement")
	}
}

func TestValidateOptions_MatchingTypeBlockOK(t *testing.T) {
	m := validDNSModel(t)
	m.DNS = &dnsOptionsModel{QueryType: strVal("AAAA"), Protocol: strVal("TCP")}
	if diags := validateMeasurement(m); diags.HasError() {
		t.Errorf("expected no error, got: %v", diags)
	}
}

func TestValidateOptions_BadEnums(t *testing.T) {
	m := validDNSModel(t)
	m.DNS = &dnsOptionsModel{Protocol: strVal("QUIC"), QueryClass: strVal("HS")}
	d := validateMeasurement(m)
	if !hasError(d, "dns.protocol") || !hasError(d, "dns.query_class") {
		t.Errorf("expected enum errors, got: %v", d)
	}

	m = validDNSModel(t)
	m.MsmType = strVal("traceroute")
	m.Traceroute = &tracerouteOptionsModel{Protocol: strVal("SCTP")}
	if !hasError(validateMeasurement(m), "traceroute.protocol") {
		t.Error("expected traceroute.protocol error")
	}
}

func TestValidateOptions_CommonTimesAndSimilarity(t *testing.T) {
	m := validDNSModel(t)
	m.Common = &commonOptionsModel{
		StartTime:         strVal("2026-12-02T00:00:00Z"),
		StopTime:          strVal("2026-12-01T00:00:00Z"),
		AutoTopupPrbSimil: types.Float64Value(1.5),
	}
	d := validateMeasurement(m)
	if !hasError(d, "stop_time must be after start_time") || !hasError(d, "between 0 and 1") {
		t.Errorf("expected time and similarity errors, got: %v", d)
	}

	m.Common = &commonOptionsModel{StartTime: strVal("tomorrow")}
	if !hasError(validateMeasurement(m), "RFC 3339") {
		t.Error("expected RFC 3339 error")
	}
}

func TestBuildMsmOptions(t *testing.T) {
	m := validDNSModel(t)
	if o := buildMsmOptions(m); o.Common.Spread != 0 || o.DNS.QueryType != "" {
		t.Errorf("nil blocks should yield zero options, got %+v", o)
	}
	m.Common = &commonOptionsModel{ResolveOnProbe: types.BoolValue(true), Spread: int64Val(30)}
	m.DNS = &dnsOptionsModel{QueryType: strVal("MX"), SetRDBit: types.BoolValue(true), Timeout: int64Val(5000)}
	o := buildMsmOptions(m)
	if !o.Common.ResolveOnProbe || o.Common.Spread != 30 {
		t.Errorf("common not mapped: %+v", o.Common)
	}
	if o.DNS.QueryType != "MX" || !o.DNS.SetRDBit || o.DNS.Timeout != 5000 {
		t.Errorf("dns not mapped: %+v", o.DNS)
	}
}

func TestSchema_Valid(t *testing.T) {
	var resp resource.SchemaResponse
	NewMeasurementResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)
	if diags := resp.Schema.ValidateImplementation(context.Background()); diags.HasError() {
		t.Fatalf("invalid schema: %v", diags)
	}
	for _, name := range []string{"common", "ping", "traceroute", "dns", "tls", "http"} {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
}
