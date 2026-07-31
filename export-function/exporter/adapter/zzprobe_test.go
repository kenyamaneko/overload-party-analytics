//go:build probe

package adapter

import (
	"context"
	"testing"

	"cloud.google.com/go/bigquery"
	"github.com/stretchr/testify/require"
)

func TestProbeFlow(t *testing.T) {
	ctx := context.Background()
	c, err := bigquery.NewClient(ctx, "test")
	require.NoError(t, err)
	defer c.Close()

	run := func(q string) {
		job, err := c.Query(q).Run(ctx)
		require.NoError(t, err, q)
		st, err := job.Wait(ctx)
		require.NoError(t, err, q)
		require.NoError(t, st.Err(), q)
	}

	run("DROP TABLE IF EXISTS analytics.t0")
	run("DROP TABLE IF EXISTS analytics.t0_staging")
	run("CREATE TABLE analytics.t0 (k1 STRING, k2 STRING, v1 STRING, v2 STRING)")
	run("CREATE TABLE analytics.t0_staging (k1 STRING, k2 STRING, v1 STRING, v2 STRING)")
	run("INSERT INTO analytics.t0 VALUES ('a','x','o1','o2'),('b','x','keep1','keep2')")
	run("INSERT INTO analytics.t0_staging VALUES ('a','z','n1','n2'),('c','z','i1','i2')")

	merge := buildMergeSQL("analytics", "t0", "t0_staging", []string{"k1"}, []string{"k1", "k2", "v1", "v2"})
	run(merge)
	run(merge) // idempotency re-run

	cit, err := c.Query("SELECT COUNT(*) FROM analytics.t0").Read(ctx)
	require.NoError(t, err)
	var crow []bigquery.Value
	require.NoError(t, cit.Next(&crow))
	n := crow[0].(int64)

	it, err := c.Query("SELECT k1,k2,v1,v2 FROM analytics.t0").Read(ctx)
	require.NoError(t, err)
	got := make([][4]string, 0, n)
	for i := int64(0); i < n; i++ {
		var row []bigquery.Value
		require.NoError(t, it.Next(&row))
		got = append(got, [4]string{row[0].(string), row[1].(string), row[2].(string), row[3].(string)})
	}
	t.Logf("rows=%v", got)
	require.ElementsMatch(t, [][4]string{
		{"a", "z", "n1", "n2"},
		{"b", "x", "keep1", "keep2"},
		{"c", "z", "i1", "i2"},
	}, got)
}
