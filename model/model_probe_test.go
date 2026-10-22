package model

import (
	"context"
	"testing"
)

func TestModelProbePersistenceAndBindingLifecycle(t *testing.T) {
	db, channels := schedulingTestSetup(t)
	ctx := context.Background()
	route, err := GetModelChannelRoute(ctx, channels[0].Id, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	result := ModelProbeResult{At: 1000, Success: true, Latency: 1234, Message: "测试成功"}
	if err := SaveModelChannelProbe(ctx, route.ID, result); err != nil {
		t.Fatal(err)
	}
	// Success must be replaceable with failure (including false and zero values).
	result = ModelProbeResult{At: 2000, Success: false, Latency: 0, Message: "测试失败"}
	if err := SaveModelChannelProbe(ctx, route.ID, result); err != nil {
		t.Fatal(err)
	}
	if err := SaveModelChannelProbe(ctx, route.ID, ModelProbeResult{At: 1500, Success: true}); err != nil {
		t.Fatal(err)
	}
	if err := SyncGatewayChannels(db, []int{channels[0].Id}); err != nil {
		t.Fatal(err)
	}
	// Startup migrations and catalog reloads preserve the last completed result.
	if err := AutoMigrateGatewayResources(db); err != nil {
		t.Fatal(err)
	}
	catalog, err := GetExplorerModels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range catalog {
		for _, r := range m.Routes {
			if r.ID == channels[0].Id && m.ID == "alpha" {
				if r.LastProbe == nil || *r.LastProbe != result {
					t.Fatal("probe lost or overwritten", r.LastProbe)
				}
			} else if r.LastProbe != nil {
				t.Fatal("probe leaked to another binding", m.ID, r.ID)
			}
		}
	}
	// A deleted/recreated binding must not inherit an old probe still in flight.
	if err := db.Delete(route).Error; err != nil {
		t.Fatal(err)
	}
	if err := SyncGatewayChannels(db, []int{channels[0].Id}); err != nil {
		t.Fatal(err)
	}
	if err := SaveModelChannelProbe(ctx, route.ID, ModelProbeResult{At: 3000, Success: true}); err != nil {
		t.Fatal(err)
	}
	recreated, err := GetModelChannelRoute(ctx, channels[0].Id, "alpha")
	if err != nil || recreated.LastProbe.At != 0 {
		t.Fatal("recreated route inherited old probe", recreated, err)
	}
}
