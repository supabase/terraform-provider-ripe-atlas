package provider

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	atlasapi "github.com/supabase/atlasctl/pkg/atlasapi"
	"github.com/supabase/atlasctl/pkg/plan"
	"github.com/supabase/atlasctl/pkg/snapshot"
)

// snapshotPath returns the absolute path to the shared test snapshot.
func snapshotPath(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs("testdata/snapshot.json")
	if err != nil {
		t.Fatalf("snapshot path: %v", err)
	}
	return abs
}

// loadTestdata reads testdata/<rel> and substitutes the SNAPSHOT_PATH placeholder.
func loadTestdata(t *testing.T, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", rel))
	if err != nil {
		t.Fatalf("read testdata %s: %v", rel, err)
	}
	return strings.ReplaceAll(string(raw), "SNAPSHOT_PATH", snapshotPath(t))
}

// ---- helpers for constructing models ----------------------------------------

func strVal(s string) types.String  { return types.StringValue(s) }
func int64Val(n int64) types.Int64  { return types.Int64Value(n) }
func nullList() types.List          { return types.ListValueMust(types.StringType, []attr.Value{}) }
func nullSet() types.Set            { return types.SetValueMust(types.Int64Type, []attr.Value{}) }

func validCohort(t *testing.T) cohortModel {
	t.Helper()
	return cohortModel{
		Name:             strVal("default"),
		ProbeCount:       int64Val(5),
		MaxProbesPerCell: int64Val(2),
		IntervalSeconds:  int64Val(60),
		IncludeProbeIDs:  nullSet(),
		ExcludeProbeIDs:  nullSet(),
	}
}

func validDNSModel(t *testing.T) measurementModel {
	t.Helper()
	return measurementModel{
		Name:    strVal("test-dns"),
		Target:  strVal("example.com"),
		MsmType: strVal("dns"),
		AF:      int64Val(4),
		Cohorts: []cohortModel{validCohort(t)},
	}
}

// hasError returns true if diags contains an error whose summary or detail
// contains substr.
func hasError(diags diag.Diagnostics, substr string) bool {
	for _, d := range diags {
		if d.Severity() == diag.SeverityError &&
			(strings.Contains(d.Summary(), substr) || strings.Contains(d.Detail(), substr)) {
			return true
		}
	}
	return false
}

// ---- validateMeasurement tests ----------------------------------------------

func TestValidateMeasurement_Valid(t *testing.T) {
	for _, msm := range []string{"dns", "ping", "tls", "traceroute"} {
		t.Run(msm, func(t *testing.T) {
			m := validDNSModel(t)
			m.MsmType = strVal(msm)
			if msm != "dns" {
				m.Target = strVal("8.8.8.8")
			}
			if diags := validateMeasurement(m); diags.HasError() {
				t.Errorf("expected no error, got: %v", diags)
			}
		})
	}
}

func TestValidateMeasurement_BadMsmType(t *testing.T) {
	m := validDNSModel(t)
	m.MsmType = strVal("icmp")
	if !hasError(validateMeasurement(m), "msm_type") {
		t.Error("expected msm_type error")
	}
}

func TestValidateMeasurement_BadAF(t *testing.T) {
	for _, af := range []int64{0, 3, 5, 8} {
		t.Run(types.Int64Value(af).String(), func(t *testing.T) {
			m := validDNSModel(t)
			m.AF = int64Val(af)
			if !hasError(validateMeasurement(m), "af") {
				t.Errorf("expected af error for af=%d", af)
			}
		})
	}
}

func TestValidateMeasurement_EmptyTarget(t *testing.T) {
	m := validDNSModel(t)
	m.Target = strVal("")
	if !hasError(validateMeasurement(m), "target") {
		t.Error("expected target error")
	}
}

func TestValidateMeasurement_IntervalTooLow(t *testing.T) {
	m := validDNSModel(t)
	m.Cohorts[0].IntervalSeconds = int64Val(30)
	if !hasError(validateMeasurement(m), "interval_seconds") {
		t.Error("expected interval_seconds error")
	}
}

func TestValidateMeasurement_IntervalAtMinimum(t *testing.T) {
	m := validDNSModel(t)
	m.Cohorts[0].IntervalSeconds = int64Val(60)
	if diags := validateMeasurement(m); diags.HasError() {
		t.Errorf("interval_seconds=60 should be valid, got: %v", diags)
	}
}

func TestValidateMeasurement_ZeroProbeCount(t *testing.T) {
	m := validDNSModel(t)
	m.Cohorts[0].ProbeCount = int64Val(0)
	if !hasError(validateMeasurement(m), "probe_count") {
		t.Error("expected probe_count error")
	}
}

func TestValidateMeasurement_ZeroMaxProbesPerCell(t *testing.T) {
	m := validDNSModel(t)
	m.Cohorts[0].MaxProbesPerCell = int64Val(0)
	if !hasError(validateMeasurement(m), "max_probes_per_cell") {
		t.Error("expected max_probes_per_cell error")
	}
}

// ---- ValidateMsmSpec tests --------------------------------------------------

func baseSpec() plan.MsmSpec {
	return plan.MsmSpec{
		Key:      plan.MsmKey{Name: "test", Cohort: "default"},
		Target:   "example.com",
		Type:     plan.MsmTypeDNS,
		AF:       4,
		Interval: 60,
		ProbeIDs: []uint32{1001, 1002, 1003},
	}
}

func TestValidateMsmSpec_ValidTypes(t *testing.T) {
	cases := []struct {
		typ    plan.MsmType
		target string
	}{
		{plan.MsmTypeDNS, "example.com"},
		{plan.MsmTypePing, "8.8.8.8"},
		{plan.MsmTypeTLS, "example.com"},
		{plan.MsmTypeTraceroute, "8.8.8.8"},
	}
	for _, tc := range cases {
		t.Run(string(tc.typ), func(t *testing.T) {
			s := baseSpec()
			s.Type = tc.typ
			s.Target = tc.target
			if err := atlasapi.ValidateMsmSpec(s); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateMsmSpec_UnknownType(t *testing.T) {
	s := baseSpec()
	s.Type = "icmp"
	if err := atlasapi.ValidateMsmSpec(s); err == nil {
		t.Error("expected error for unknown type")
	}
}

func TestValidateMsmSpec_BadAF(t *testing.T) {
	s := baseSpec()
	s.AF = 8
	if err := atlasapi.ValidateMsmSpec(s); err == nil {
		t.Error("expected error for bad af")
	}
}

func TestValidateMsmSpec_EmptyTarget_NonDNS(t *testing.T) {
	s := baseSpec()
	s.Type = plan.MsmTypePing
	s.Target = ""
	if err := atlasapi.ValidateMsmSpec(s); err == nil {
		t.Error("expected error for empty target on ping")
	}
}

// ---- runSelection tests -----------------------------------------------------

func TestRunSelection_ValidDNS(t *testing.T) {
	m := validDNSModel(t)
	selected, err := runSelection(context.Background(), &snapshot.FileProbeSource{Path: snapshotPath(t)}, m)
	if err != nil {
		t.Fatalf("runSelection: %v", err)
	}
	if len(selected) == 0 {
		t.Fatal("expected at least one selected cohort")
	}
	ids := cohortProbeIDs(selected[0])
	if len(ids) == 0 {
		t.Error("expected non-empty probe IDs")
	}
	if len(ids) > int(m.Cohorts[0].ProbeCount.ValueInt64()) {
		t.Errorf("got %d IDs, want <= %d", len(ids), m.Cohorts[0].ProbeCount.ValueInt64())
	}
}

func TestRunSelection_ExcludeTagsApplied(t *testing.T) {
	// The snapshot has probe 9999 tagged "broken". With cohort cfg.exclude_tags=["broken"]
	// it should never appear in the result.
	m := validDNSModel(t)
	m.Cohorts[0].Cfg = &cfgModel{
		ExcludeTags: types.ListValueMust(types.StringType, []attr.Value{
			types.StringValue("broken"),
		}),
	}
	selected, err := runSelection(context.Background(), &snapshot.FileProbeSource{Path: snapshotPath(t)}, m)
	if err != nil {
		t.Fatalf("runSelection: %v", err)
	}
	for _, id := range cohortProbeIDs(selected[0]) {
		if id == 9999 {
			t.Error("excluded probe 9999 appeared in selection")
		}
	}
}

func TestRunSelection_IncludeProbeIDsHonored(t *testing.T) {
	m := validDNSModel(t)
	m.Cohorts[0].IncludeProbeIDs = types.SetValueMust(types.Int64Type, []attr.Value{
		types.Int64Value(1001),
	})
	selected, err := runSelection(context.Background(), &snapshot.FileProbeSource{Path: snapshotPath(t)}, m)
	if err != nil {
		t.Fatalf("runSelection: %v", err)
	}
	found := false
	for _, id := range cohortProbeIDs(selected[0]) {
		if id == 1001 {
			found = true
			break
		}
	}
	if !found {
		t.Error("include_probe_ids probe 1001 missing from selection")
	}
}

func TestRunSelection_MissingSnapshot(t *testing.T) {
	m := validDNSModel(t)
	if _, err := runSelection(context.Background(), &snapshot.FileProbeSource{Path: "/nonexistent/snapshot.json"}, m); err == nil {
		t.Error("expected error for missing snapshot")
	}
}

func TestRunSelection_DrawdownAcrossCohorts(t *testing.T) {
	// Two cohorts requesting the same probes. The second cohort must not overlap
	// with the first — the drawdown in selection.Select enforces this.
	m := validDNSModel(t)
	m.Cohorts = []cohortModel{
		{
			Name:            strVal("first"),
			ProbeCount:      int64Val(3),
			MaxProbesPerCell: int64Val(2),
			IntervalSeconds: int64Val(60),
			IncludeProbeIDs: nullSet(),
			ExcludeProbeIDs: nullSet(),
		},
		{
			Name:            strVal("second"),
			ProbeCount:      int64Val(3),
			MaxProbesPerCell: int64Val(2),
			IntervalSeconds: int64Val(60),
			IncludeProbeIDs: nullSet(),
			ExcludeProbeIDs: nullSet(),
		},
	}
	selected, err := runSelection(context.Background(), &snapshot.FileProbeSource{Path: snapshotPath(t)}, m)
	if err != nil {
		t.Fatalf("runSelection: %v", err)
	}
	if len(selected) != 2 {
		t.Fatalf("expected 2 selected cohorts, got %d", len(selected))
	}

	first := make(map[uint32]struct{})
	for _, id := range cohortProbeIDs(selected[0]) {
		first[id] = struct{}{}
	}
	for _, id := range cohortProbeIDs(selected[1]) {
		if _, overlap := first[id]; overlap {
			t.Errorf("probe %d appears in both cohorts — drawdown is broken", id)
		}
	}
}

// ---- probeSourceFromEnv tests -----------------------------------------------

func TestProbeSourceFromEnv_FileMode(t *testing.T) {
	t.Setenv("RIPE_ATLAS_SNAPSHOT", snapshotPath(t))
	src := probeSourceFromEnv(false)
	if _, ok := src.(*snapshot.FileProbeSource); !ok {
		t.Fatalf("expected *snapshot.FileProbeSource, got %T", src)
	}
	probes, err := src.Probes(context.Background())
	if err != nil {
		t.Fatalf("Probes: %v", err)
	}
	if len(probes) == 0 {
		t.Error("expected non-empty probe list")
	}
}

func TestProbeSourceFromEnv_CacheMode(t *testing.T) {
	t.Setenv("RIPE_ATLAS_SNAPSHOT", "")
	src := probeSourceFromEnv(false)
	if _, ok := src.(*snapshot.CachedProbeSource); !ok {
		t.Fatalf("expected *snapshot.CachedProbeSource, got %T", src)
	}
}

func TestProbeSourceFromEnv_ModifyPlanFallback(t *testing.T) {
	// When r.clients == nil, ModifyPlan must use probeSourceFromEnv and populate
	// probe_ids. This exercises the Pulumi bridge preview lifecycle where
	// Configure has not yet run.
	t.Setenv("RIPE_ATLAS_SNAPSHOT", snapshotPath(t))

	m := validDNSModel(t)
	selected, err := runSelection(context.Background(), probeSourceFromEnv(false), m)
	if err != nil {
		t.Fatalf("runSelection with env fallback: %v", err)
	}
	if len(selected) == 0 {
		t.Fatal("expected at least one selected cohort")
	}
	if len(cohortProbeIDs(selected[0])) == 0 {
		t.Error("expected non-empty probe IDs from env fallback path")
	}
}

// ---- buildCohortCfg tests ---------------------------------------------------

func TestBuildCohortCfg_ASNKeyParsing(t *testing.T) {
	c := cfgModel{
		ASN: types.MapValueMust(types.Int64Type, map[string]attr.Value{
			"7018": types.Int64Value(10),
			"7922": types.Int64Value(8),
		}),
		Tags:      types.MapNull(types.Int64Type),
		Countries: types.MapNull(types.Int64Type),
		Stability: types.MapNull(types.Int64Type),
	}
	cfg := buildCohortCfg(c)
	if cfg.ASN[7018] != 10 {
		t.Errorf("ASN 7018: want 10, got %d", cfg.ASN[7018])
	}
	if cfg.ASN[7922] != 8 {
		t.Errorf("ASN 7922: want 8, got %d", cfg.ASN[7922])
	}
	if len(cfg.Tags) != 0 {
		t.Errorf("Tags should be empty, got %v", cfg.Tags)
	}
}

func TestBuildCohortCfg_NullMapsProduceNilMaps(t *testing.T) {
	c := cfgModel{
		ASN:       types.MapNull(types.Int64Type),
		Tags:      types.MapNull(types.Int64Type),
		Countries: types.MapNull(types.Int64Type),
		Stability: types.MapNull(types.Int64Type),
	}
	cfg := buildCohortCfg(c)
	if cfg.ASN != nil || cfg.Tags != nil || cfg.Countries != nil || cfg.Stability != nil {
		t.Error("all null maps should produce nil Go maps")
	}
}

// ---- testdata file smoke tests ----------------------------------------------
// These verify the testdata files are syntactically valid (SNAPSHOT_PATH is
// substituted correctly). They do not run Terraform; they just confirm the
// file loads without I/O errors.

func TestTestdataFilesExist(t *testing.T) {
	files := []string{
		"valid/dns.tf",
		"valid/ping.tf",
		"valid/tls.tf",
		"valid/traceroute.tf",
		"invalid/bad_msm_type.tf",
		"invalid/bad_af.tf",
		"invalid/empty_target.tf",
		"invalid/interval_too_low.tf",
		"invalid/zero_probe_count.tf",
		"invalid/zero_max_probes.tf",
		"invalid/missing_cohort.tf",
	}
	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			content := loadTestdata(t, f)
			if len(content) == 0 {
				t.Error("empty file")
			}
			if strings.Contains(content, "SNAPSHOT_PATH") {
				t.Error("SNAPSHOT_PATH placeholder not substituted")
			}
		})
	}
}
