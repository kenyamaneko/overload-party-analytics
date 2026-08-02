//go:build integration

package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"export-to-bq/exporter/model"
)

// workspaceDirEnv は各サービスリポジトリが並ぶディレクトリを指す環境変数です。
// 未設定のときは本リポジトリの一つ上の階層を使います。
const workspaceDirEnv = "OVERLOAD_PARTY_WORKSPACE_DIR"

const (
	postgresImage      = "postgres:16-alpine"
	containerStartWait = 60 * time.Second
)

// sourceSQLFiles はエクスポート元の実スキーマとマスタデータの投入 SQL を、適用順に並べたものです。
// gateway は battle.games を参照し、seed はスキーマの後に適用する必要があります。
var sourceSQLFiles = []string{
	"overload-party-account/db/schema.sql",
	"overload-party-battle/db/schema.sql",
	"overload-party-gateway/db/schema.sql",
	"overload-party-card/db/schema.sql",
	"overload-party-shop/db/schema.sql",
	"overload-party-card/db/seed/cards_seed.sql",
}

// テストが投入する固定値。実データの ID と衝突しないダミー値を使う。
const (
	tstGameID      = "TSTGAME0000000000000000001"
	tstPlayerID    = "00000000-0000-0000-0000-0000000000a1"
	tstDeckOwnerID = "00000000-0000-0000-0000-0000000000d1"
	tstFirebaseUID = "tst-firebase-uid"
	tstProductID   = "tst_product"
	tstCardA       = "TST-0001"
	tstCardB       = "TST-0002"
	tstCardC       = "TST-0003"
)

var (
	tstFixtureTime = time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	tstDeckTimeV1  = time.Date(2025, 7, 1, 9, 0, 0, 0, time.UTC)
)

var (
	sharedDSN  string
	sharedPool *pgxpool.Pool
	sharedConf *model.Config
)

func TestMain(m *testing.M) {
	os.Exit(runIntegrationTests(m))
}

func runIntegrationTests(m *testing.M) int {
	ctx := context.Background()

	container, err := postgres.Run(ctx,
		postgresImage,
		postgres.WithDatabase("test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(containerStartWait),
		),
	)
	if err != nil {
		panic(fmt.Sprintf("start postgres container: %v", err))
	}
	defer func() {
		if terr := container.Terminate(ctx); terr != nil {
			panic(fmt.Sprintf("terminate postgres container: %v", terr))
		}
	}()

	sharedDSN, err = container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		panic(fmt.Sprintf("container connection string: %v", err))
	}

	sharedPool, err = pgxpool.New(ctx, sharedDSN)
	if err != nil {
		panic(fmt.Sprintf("create setup pool: %v", err))
	}
	defer sharedPool.Close()

	if err := applySourceSQL(ctx, sharedPool); err != nil {
		panic(err.Error())
	}
	if err := insertFixtures(ctx, sharedPool); err != nil {
		panic(fmt.Sprintf("insert fixtures: %v", err))
	}

	sharedConf, err = model.LoadConfig(filepath.Join(moduleDir(), "config.yaml"))
	if err != nil {
		panic(fmt.Sprintf("load config: %v", err))
	}

	return m.Run()
}

// applySourceSQL は各サービスリポジトリが所有する DDL とマスタデータをそのまま適用します。
func applySourceSQL(ctx context.Context, pool *pgxpool.Pool) error {
	root, err := workspaceDir()
	if err != nil {
		return err
	}
	for _, rel := range sourceSQLFiles {
		path := filepath.Join(root, rel)
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return fmt.Errorf("read %s: %w (set %s to the directory holding the service repositories)", path, rerr, workspaceDirEnv)
		}
		if _, eerr := pool.Exec(ctx, string(data)); eerr != nil {
			return fmt.Errorf("apply %s: %w", path, eerr)
		}
	}
	return nil
}

func insertFixtures(ctx context.Context, pool *pgxpool.Pool) error {
	statements := []struct {
		sql  string
		args []interface{}
	}{
		{
			sql: `INSERT INTO account.players
			        (player_id, firebase_uid, name, is_premium, equipped_icon_no,
			         onboarding_status, premium_expires_at, created_at, updated_at)
			      VALUES ($1, $2, 'tst-player', true, 3, 'completed', $3, $3, $3)`,
			args: []interface{}{tstPlayerID, tstFirebaseUID, tstFixtureTime},
		},
		{
			sql: `INSERT INTO battle.games
			        (game_id, status, first_player, winning_player_num, win_reason,
			         engine_version, card_data_version, created_at, updated_at, finished_at)
			      VALUES ($1, 'finished', 1, 2, 'budget_zero', 'tst-engine', 'tst-cards', $2, $2, $2)`,
			args: []interface{}{tstGameID, tstFixtureTime},
		},
		{
			sql: `INSERT INTO battle.game_events
			        (game_id, sequence_number, event_type, player_num, event_data, created_at)
			      VALUES ($1, 1, 'tst_event', 1, '{"tst_key": "tst_value"}'::jsonb, $2)`,
			args: []interface{}{tstGameID, tstFixtureTime},
		},
		{
			sql: `INSERT INTO gateway.game_players (game_id, player_num, player_id, exp_awarded)
			      VALUES ($1, 1, $2, true)`,
			args: []interface{}{tstGameID, tstPlayerID},
		},
		{
			sql: `INSERT INTO shop.subscriptions
			        (player_id, product_id, status, current_period_start, current_period_end, created_at, updated_at)
			      VALUES ($1, $2, 'active', $3, $3, $3, $3)`,
			args: []interface{}{tstPlayerID, tstProductID, tstFixtureTime},
		},
		{
			sql:  `INSERT INTO shop.one_time_purchases (player_id, product_id, purchased_at) VALUES ($1, $2, $3)`,
			args: []interface{}{tstPlayerID, tstProductID, tstFixtureTime},
		},
	}
	for _, stmt := range statements {
		if _, err := pool.Exec(ctx, stmt.sql, stmt.args...); err != nil {
			return fmt.Errorf("%s: %w", stmt.sql, err)
		}
	}

	deckID, err := insertDeck(ctx, pool, tstPlayerID, tstFixtureTime)
	if err != nil {
		return err
	}
	return insertDeckCards(ctx, pool, tstPlayerID, deckID, tstCardA)
}

// insertDeck はデッキヘッダーを 1 件作り、採番されたデッキ ID を返します。
func insertDeck(ctx context.Context, pool *pgxpool.Pool, playerID string, updatedAt time.Time) (int64, error) {
	var deckID int64
	err := pool.QueryRow(ctx,
		`INSERT INTO card.decks
		   (player_id, deck_name, faction, product_id, routine_id, special_id,
		    playmat_no, sleeve_no, created_at, updated_at)
		 VALUES ($1, 'tst-deck', 'SHE', 'TSTP-001', 'TSTR-001', 'TSTS-001', 1, 1, $2, $2)
		 RETURNING deck_id`,
		playerID, updatedAt,
	).Scan(&deckID)
	if err != nil {
		return 0, fmt.Errorf("insert deck: %w", err)
	}
	return deckID, nil
}

func insertDeckCards(ctx context.Context, pool *pgxpool.Pool, playerID string, deckID int64, cardIDs ...string) error {
	for _, cardID := range cardIDs {
		if _, err := pool.Exec(ctx,
			`INSERT INTO card.deck_cards (player_id, deck_id, card_id, art_no, count)
			 VALUES ($1, $2, $3, 1, 3)`,
			playerID, deckID, cardID,
		); err != nil {
			return fmt.Errorf("insert deck card %s: %w", cardID, err)
		}
	}
	return nil
}

// workspaceDir は各サービスリポジトリが並ぶディレクトリを返します。
func workspaceDir() (string, error) {
	if dir := os.Getenv(workspaceDirEnv); dir != "" {
		return dir, nil
	}
	return filepath.Dir(filepath.Dir(moduleDir())), nil
}

// moduleDir は go.mod を持つディレクトリを返します。
func moduleDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("runtime.Caller failed")
	}
	dir := filepath.Dir(thisFile)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			panic("go.mod not found from the integration test")
		}
		dir = parent
	}
}

func newReader(t *testing.T) *PostgresReader {
	t.Helper()
	reader, err := NewPostgresReader(context.Background(), &model.Config{DatabaseConn: sharedDSN})
	require.NoError(t, err)
	t.Cleanup(reader.Close)
	return reader
}

// encodeJSONL は GCS へ書き出すのと同じ形で行を JSON にし、デコードし直した内容を返します。
func encodeJSONL(t *testing.T, rows []map[string]interface{}) []map[string]interface{} {
	t.Helper()
	decoded := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		encoded, err := json.Marshal(row)
		require.NoError(t, err)
		var parsed map[string]interface{}
		require.NoError(t, json.Unmarshal(encoded, &parsed))
		decoded = append(decoded, parsed)
	}
	return decoded
}

func TestPostgresExport(t *testing.T) {
	ctx := context.Background()
	wholeRange := func() (time.Time, time.Time) {
		return time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), time.Now().Add(time.Hour)
	}

	t.Run("実スキーマからのエクスポート", func(t *testing.T) {
		tables := []string{
			"games", "game_events", "game_players", "players",
			"card_definitions", "deck_cards", "subscriptions", "purchases",
		}
		for _, table := range tables {
			t.Run(fmt.Sprintf("%s のとき、取得した全行が JSONL に変換される", table), func(t *testing.T) {
				tableConfig, ok := sharedConf.Tables[table]
				require.True(t, ok)

				start, end := wholeRange()
				rows, err := newReader(t).Query(ctx, tableConfig, start, end)
				require.NoError(t, err)
				require.NotEmpty(t, rows)

				require.Len(t, encodeJSONL(t, rows), len(rows))
			})
		}

		t.Run("カードマスターのとき、効果定義が JSON の配列として出力される", func(t *testing.T) {
			start, end := wholeRange()
			rows, err := newReader(t).Query(ctx, sharedConf.Tables["card_definitions"], start, end)
			require.NoError(t, err)

			arrayCount := 0
			for _, row := range encodeJSONL(t, rows) {
				if row["effects"] == nil {
					continue
				}
				require.IsType(t, []interface{}{}, row["effects"], "card_id=%v", row["card_id"])
				arrayCount++
			}
			require.NotZero(t, arrayCount)
		})

		t.Run("対戦テーブルのとき、投入した対戦の内容が JSONL に現れる", func(t *testing.T) {
			start, end := wholeRange()
			rows, err := newReader(t).Query(ctx, sharedConf.Tables["games"], start, end)
			require.NoError(t, err)

			var found map[string]interface{}
			for _, row := range encodeJSONL(t, rows) {
				if row["game_id"] == tstGameID {
					found = row
				}
			}
			require.NotNil(t, found)
			require.Equal(t, "finished", found["status"])
			require.Equal(t, float64(1), found["first_player"])
			require.Equal(t, float64(2), found["winning_player_num"])
			require.Equal(t, "budget_zero", found["win_reason"])

			createdAt, ok := found["created_at"].(string)
			require.True(t, ok)
			parsed, err := time.Parse(time.RFC3339Nano, createdAt)
			require.NoError(t, err)
			require.Equal(t, tstFixtureTime, parsed.UTC())
		})

		t.Run("プレイヤーとゲームスロットの対応のとき、プレイヤー ID がハイフン区切りの文字列で出力される", func(t *testing.T) {
			start, end := wholeRange()
			rows, err := newReader(t).Query(ctx, sharedConf.Tables["game_players"], start, end)
			require.NoError(t, err)

			decoded := encodeJSONL(t, rows)
			require.Len(t, decoded, 1)
			require.Equal(t, tstPlayerID, decoded[0]["player_id"])
		})

		t.Run("デッキからカードを外して別のカードを入れると、編集後の時間範囲では編集後の構成だけが同じ更新日時で出力される", func(t *testing.T) {
			deckID, err := insertDeck(ctx, sharedPool, tstDeckOwnerID, tstDeckTimeV1)
			require.NoError(t, err)
			require.NoError(t, insertDeckCards(ctx, sharedPool, tstDeckOwnerID, deckID, tstCardA, tstCardB))

			reader := newReader(t)
			beforeEdit, err := reader.Query(ctx, sharedConf.Tables["deck_cards"], tstDeckTimeV1, tstDeckTimeV1.Add(time.Second))
			require.NoError(t, err)
			require.ElementsMatch(t, []string{tstCardA, tstCardB}, cardIDsOf(t, beforeEdit))

			_, err = sharedPool.Exec(ctx,
				`DELETE FROM card.deck_cards WHERE player_id = $1 AND deck_id = $2 AND card_id = $3`,
				tstDeckOwnerID, deckID, tstCardB)
			require.NoError(t, err)
			require.NoError(t, insertDeckCards(ctx, sharedPool, tstDeckOwnerID, deckID, tstCardC))

			var editedAt time.Time
			require.NoError(t, sharedPool.QueryRow(ctx,
				`UPDATE card.decks SET deck_name = 'tst-deck-edited'
				 WHERE player_id = $1 AND deck_id = $2
				 RETURNING updated_at`,
				tstDeckOwnerID, deckID).Scan(&editedAt))

			afterEdit, err := reader.Query(ctx, sharedConf.Tables["deck_cards"], editedAt, editedAt.Add(time.Second))
			require.NoError(t, err)
			require.ElementsMatch(t, []string{tstCardA, tstCardC}, cardIDsOf(t, afterEdit))

			for _, row := range encodeJSONL(t, afterEdit) {
				require.Equal(t, editedAt.Format(time.RFC3339Nano), row["updated_at"], "card_id=%v", row["card_id"])
			}
		})
	})
}

func cardIDsOf(t *testing.T, rows []map[string]interface{}) []string {
	t.Helper()
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		id, ok := row["card_id"].(string)
		require.True(t, ok)
		ids = append(ids, id)
	}
	return ids
}
