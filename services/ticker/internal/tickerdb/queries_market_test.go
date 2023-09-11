package tickerdb

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	migrate "github.com/rubenv/sql-migrate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetrieveMarketData(t *testing.T) {
	db := OpenTestDBConnection(t)
	defer db.Close()

	var session TickerSession
	session.DB = db.Open()
	ctx := context.Background()
	defer session.DB.Close()

	// Run migrations to make sure the tests are run
	// on the most updated schema version
	migrations := &migrate.FileMigrationSource{
		Dir: "./migrations",
	}
	_, err := migrate.Exec(session.DB.DB, "postgres", migrations, migrate.Up)
	require.NoError(t, err)

	// Adding a seed issuer to be used later:
	tbl := session.GetTable("issuers")
	_, err = tbl.Insert(Issuer{
		PublicKey: "GCF3TQXKZJNFJK7HCMNE2O2CUNKCJH2Y2ROISTBPLC7C5EIA5NNG2XZB",
		Name:      "FOO BAR",
	}).IgnoreCols("id").Exec(ctx)
	require.NoError(t, err)
	var issuer Issuer
	err = session.GetRaw(ctx, &issuer, `
		SELECT *
		FROM issuers
		ORDER BY id DESC
		LIMIT 1`,
	)
	require.NoError(t, err)

	// Adding a seed asset to be used later:
	err = session.InsertOrUpdateAsset(ctx, &Asset{
		Code:     "GRAM",
		IssuerID: issuer.ID,
		IsValid:  true,
	}, []string{"code", "issuer_id"})
	require.NoError(t, err)
	var gramAsset Asset
	err = session.GetRaw(ctx, &gramAsset, `
		SELECT *
		FROM assets
		ORDER BY id DESC
		LIMIT 1`,
	)
	require.NoError(t, err)

	// Adding another asset to be used later:
	err = session.InsertOrUpdateAsset(ctx, &Asset{
		Code:     "BTC",
		IssuerID: issuer.ID,
		IsValid:  true,
	}, []string{"code", "issuer_id"})
	require.NoError(t, err)
	var btcAsset Asset
	err = session.GetRaw(ctx, &btcAsset, `
		SELECT *
		FROM assets
		ORDER BY id DESC
		LIMIT 1`,
	)
	require.NoError(t, err)

	// Adding a third asset:
	err = session.InsertOrUpdateAsset(ctx, &Asset{
		Code:     "ETH",
		IssuerID: issuer.ID,
		IsValid:  true,
	}, []string{"code", "issuer_id"})
	require.NoError(t, err)
	var ethAsset Asset
	err = session.GetRaw(ctx, &ethAsset, `
		SELECT *
		FROM assets
		ORDER BY id DESC
		LIMIT 1`,
	)
	require.NoError(t, err)

	// Verify that we actually have three assets:
	assert.NotEqual(t, gramAsset.ID, btcAsset.ID)
	assert.NotEqual(t, btcAsset.ID, ethAsset.ID)
	assert.NotEqual(t, gramAsset.ID, ethAsset.ID)

	// A few times to be used:
	now := time.Now()
	oneHourAgo := now.Add(-1 * time.Hour)
	threeDaysAgo := now.AddDate(0, 0, -3)
	oneMonthAgo := now.AddDate(0, -1, 0)

	// Now let's create the trades:
	trades := []Trade{
		{ // GRAM_BTC trade
			OrbitRID:       "hrzid1",
			BaseAssetID:     gramAsset.ID,
			BaseAmount:      100.0,
			CounterAssetID:  btcAsset.ID,
			CounterAmount:   10.0,
			Price:           0.1,
			LedgerCloseTime: now,
		},
		{ // GRAM_ETH trade
			OrbitRID:       "hrzid3",
			BaseAssetID:     gramAsset.ID,
			BaseAmount:      24.0,
			CounterAssetID:  ethAsset.ID,
			CounterAmount:   26.0,
			Price:           0.92,
			LedgerCloseTime: oneHourAgo,
		},
		{ // GRAM_ETH trade
			OrbitRID:       "hrzid2",
			BaseAssetID:     gramAsset.ID,
			BaseAmount:      50.0,
			CounterAssetID:  ethAsset.ID,
			CounterAmount:   50.0,
			Price:           1.0,
			LedgerCloseTime: now,
		},
		{ // GRAM_BTC trade
			OrbitRID:       "hrzid4",
			BaseAssetID:     gramAsset.ID,
			BaseAmount:      50.0,
			CounterAssetID:  btcAsset.ID,
			CounterAmount:   6.0,
			Price:           0.12,
			LedgerCloseTime: threeDaysAgo,
		},
		{ // GRAM_ETH trade
			OrbitRID:       "hrzid5",
			BaseAssetID:     gramAsset.ID,
			BaseAmount:      24.0,
			CounterAssetID:  ethAsset.ID,
			CounterAmount:   28.0,
			Price:           1.10,
			LedgerCloseTime: oneMonthAgo,
		},
	}
	err = session.BulkInsertTrades(ctx, trades)
	require.NoError(t, err)

	// Adding some orderbook stats:
	obTime := time.Now()
	orderbookStats := OrderbookStats{
		BaseAssetID:    gramAsset.ID,
		CounterAssetID: ethAsset.ID,
		NumBids:        15,
		BidVolume:      0.15,
		HighestBid:     200.0,
		NumAsks:        17,
		AskVolume:      30.0,
		LowestAsk:      0.1,
		Spread:         0.93,
		SpreadMidPoint: 0.35,
		UpdatedAt:      obTime,
	}
	err = session.InsertOrUpdateOrderbookStats(ctx,
		&orderbookStats,
		[]string{"base_asset_id", "counter_asset_id"},
	)
	require.NoError(t, err)

	var obBTCETH1 OrderbookStats
	err = session.GetRaw(ctx, &obBTCETH1, `
		SELECT *
		FROM orderbook_stats
		ORDER BY id DESC
		LIMIT 1`,
	)
	require.NoError(t, err)

	orderbookStats = OrderbookStats{
		BaseAssetID:    gramAsset.ID,
		CounterAssetID: btcAsset.ID,
		NumBids:        1,
		BidVolume:      0.1,
		HighestBid:     20.0,
		NumAsks:        1,
		AskVolume:      15.0,
		LowestAsk:      0.2,
		Spread:         0.96,
		SpreadMidPoint: 0.36,
		UpdatedAt:      obTime,
	}
	err = session.InsertOrUpdateOrderbookStats(ctx,
		&orderbookStats,
		[]string{"base_asset_id", "counter_asset_id"},
	)
	require.NoError(t, err)

	var obBTCETH2 OrderbookStats
	err = session.GetRaw(ctx, &obBTCETH2, `
		SELECT *
		FROM orderbook_stats
		ORDER BY id DESC
		LIMIT 1`,
	)
	require.NoError(t, err)
	assert.NotEqual(t, obBTCETH1.ID, obBTCETH2.ID)

	markets, err := session.RetrieveMarketData(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, len(markets))

	// Mapping the retrieved markets:
	var grambtcMkt, gramethMkt Market
	for _, mkt := range markets {
		if mkt.TradePair == "GRAM_BTC" {
			grambtcMkt = mkt
		}

		if mkt.TradePair == "GRAM_ETH" {
			gramethMkt = mkt
		}
	}
	assert.NotEqual(t, "", grambtcMkt.TradePair)
	assert.NotEqual(t, "", gramethMkt.TradePair)

	// Validating the aggregated data
	assert.Equal(t, 100.0, grambtcMkt.BaseVolume24h)
	assert.Equal(t, 10.0, grambtcMkt.CounterVolume24h)
	assert.Equal(t, int64(1), grambtcMkt.TradeCount24h)
	assert.Equal(t, 0.1, grambtcMkt.OpenPrice24h)
	assert.Equal(t, 0.1, grambtcMkt.LowestPrice24h)
	assert.Equal(t, 0.1, grambtcMkt.HighestPrice24h)

	assert.Equal(t, 150.0, grambtcMkt.BaseVolume7d)
	assert.Equal(t, 16.0, grambtcMkt.CounterVolume7d)
	assert.Equal(t, int64(2), grambtcMkt.TradeCount7d)
	assert.Equal(t, 0.12, grambtcMkt.OpenPrice7d)
	assert.Equal(t, 0.1, grambtcMkt.LowestPrice7d)
	assert.Equal(t, 0.12, grambtcMkt.HighestPrice7d)

	assert.Equal(t, 0.1, grambtcMkt.LastPrice)
	assert.WithinDuration(t, now.Local(), grambtcMkt.LastPriceCloseTime.Local(), 10*time.Millisecond)

	assert.Equal(t, 0.0, grambtcMkt.PriceChange24h)
	// There might be some floating point rounding issues, so this test
	// needs to be a bit more flexible. Since the change is 0.02, an error
	// around 0.0000000000001 is acceptable:
	priceChange7dDiff := math.Abs(-0.02 - grambtcMkt.PriceChange7d)
	assert.True(t, priceChange7dDiff < 0.0000000000001)

	assert.Equal(t, 74.0, gramethMkt.BaseVolume24h)
	assert.Equal(t, 76.0, gramethMkt.CounterVolume24h)
	assert.Equal(t, int64(2), gramethMkt.TradeCount24h)
	assert.Equal(t, 0.92, gramethMkt.OpenPrice24h)
	assert.Equal(t, 0.92, gramethMkt.LowestPrice24h)
	assert.Equal(t, 1.0, gramethMkt.HighestPrice24h)

	assert.Equal(t, 74.0, gramethMkt.BaseVolume7d)
	assert.Equal(t, 76.0, gramethMkt.CounterVolume7d)
	assert.Equal(t, int64(2), gramethMkt.TradeCount7d)
	assert.Equal(t, 0.92, gramethMkt.OpenPrice7d)
	assert.Equal(t, 0.92, gramethMkt.LowestPrice7d)
	assert.Equal(t, 1.0, gramethMkt.HighestPrice7d)

	assert.Equal(t, 1.0, gramethMkt.LastPrice)
	assert.WithinDuration(t, now.Local(), grambtcMkt.LastPriceCloseTime.Local(), 10*time.Millisecond)

	// There might be some floating point rounding issues, so this test
	// needs to be a bit more flexible. Since the change is 0.08, an error
	// around 0.0000000000001 is acceptable:
	priceChange24hDiff := math.Abs(0.08 - gramethMkt.PriceChange24h)
	assert.True(t, priceChange24hDiff < 0.0000000000001)

	priceChange7dDiff = math.Abs(0.08 - gramethMkt.PriceChange7d)
	assert.True(t, priceChange7dDiff < 0.0000000000001)

	assert.Equal(t, priceChange24hDiff, priceChange7dDiff)

	// Analyzing aggregated orderbook data:
	assert.Equal(t, 15, gramethMkt.NumBids)
	assert.Equal(t, 0.15, gramethMkt.BidVolume)
	assert.Equal(t, 200.0, gramethMkt.HighestBid)
	assert.Equal(t, 17, gramethMkt.NumAsks)
	assert.Equal(t, 30.0, gramethMkt.AskVolume)
	assert.Equal(t, 0.1, gramethMkt.LowestAsk)

	assert.Equal(t, 1, grambtcMkt.NumBids)
	assert.Equal(t, 0.1, grambtcMkt.BidVolume)
	assert.Equal(t, 20.0, grambtcMkt.HighestBid)
	assert.Equal(t, 1, grambtcMkt.NumAsks)
	assert.Equal(t, 15.0, grambtcMkt.AskVolume)
	assert.Equal(t, 0.2, grambtcMkt.LowestAsk)
}

func TestRetrievePartialMarkets(t *testing.T) {
	db := OpenTestDBConnection(t)
	defer db.Close()

	var session TickerSession
	session.DB = db.Open()
	ctx := context.Background()
	defer session.DB.Close()

	// Run migrations to make sure the tests are run
	// on the most updated schema version
	migrations := &migrate.FileMigrationSource{
		Dir: "./migrations",
	}
	_, err := migrate.Exec(session.DB.DB, "postgres", migrations, migrate.Up)
	require.NoError(t, err)

	// Adding a seed issuer to be used later:
	issuer1PK := "GCF3TQXKZJNFJK7HCMNE2O2CUNKCJH2Y2ROISTBPLC7C5EIA5NNG2XZB"
	tbl := session.GetTable("issuers")
	_, err = tbl.Insert(Issuer{
		PublicKey: issuer1PK,
		Name:      "FOO BAR",
	}).IgnoreCols("id").Exec(ctx)
	require.NoError(t, err)
	var issuer1 Issuer
	err = session.GetRaw(ctx, &issuer1, `
		SELECT *
		FROM issuers
		WHERE public_key = ?`,
		issuer1PK,
	)
	require.NoError(t, err)

	// Adding another issuer to be used later:
	issuer2PK := "ABF3TQXKZJNFJK7HCMNE2O2CUNKCJH2Y2ROISTBPLC7C5EIA5NNG2XZB"
	_, err = tbl.Insert(Issuer{
		PublicKey: issuer2PK,
		Name:      "FOO BAR",
	}).IgnoreCols("id").Exec(ctx)
	require.NoError(t, err)
	var issuer2 Issuer
	err = session.GetRaw(ctx, &issuer2, `
		SELECT *
		FROM issuers
		WHERE public_key = ?`,
		issuer2PK,
	)
	require.NoError(t, err)

	// Adding a seed asset to be used later:
	err = session.InsertOrUpdateAsset(ctx, &Asset{
		Code:          "ETH",
		IssuerAccount: issuer1PK,
		IssuerID:      issuer1.ID,
		IsValid:       true,
	}, []string{"code", "issuer_id"})
	require.NoError(t, err)
	var ethAsset1 Asset
	err = session.GetRaw(ctx, &ethAsset1, `
		SELECT *
		FROM assets
		WHERE code = ?
		AND issuer_account = ?`,
		"ETH",
		issuer1PK,
	)
	require.NoError(t, err)

	// Adding a seed asset to be used later:
	err = session.InsertOrUpdateAsset(ctx, &Asset{
		Code:          "ETH",
		IssuerAccount: issuer2PK,
		IssuerID:      issuer2.ID,
		IsValid:       true,
	}, []string{"code", "issuer_id"})
	require.NoError(t, err)
	var ethAsset2 Asset
	err = session.GetRaw(ctx, &ethAsset2, `
		SELECT *
		FROM assets
		WHERE code = ?
		AND issuer_account = ?`,
		"ETH",
		issuer2PK,
	)
	require.NoError(t, err)

	// Adding another asset to be used later:
	err = session.InsertOrUpdateAsset(ctx, &Asset{
		Code:          "BTC",
		IssuerAccount: issuer1PK,
		IssuerID:      issuer1.ID,
		IsValid:       true,
	}, []string{"code", "issuer_id"})
	require.NoError(t, err)
	var btcAsset Asset
	err = session.GetRaw(ctx, &btcAsset, `
		SELECT *
		FROM assets
		WHERE code = ?
		AND issuer_account = ?`,
		"BTC",
		issuer1PK,
	)
	require.NoError(t, err)

	// A few times to be used:
	now := time.Now()
	tenMinutesAgo := now.Add(-10 * time.Minute)
	oneHourAgo := now.Add(-1 * time.Hour)
	threeDaysAgo := now.AddDate(0, 0, -3)

	// Now let's create the trades:
	trades := []Trade{
		{ // BTC_ETH  trade (ETH is from issuer 1)
			OrbitRID:       "hrzid1",
			BaseAssetID:     btcAsset.ID,
			BaseAmount:      100.0,
			CounterAssetID:  ethAsset1.ID,
			CounterAmount:   10.0,
			Price:           0.1,
			LedgerCloseTime: tenMinutesAgo,
		},
		{ // BTC_ETH trade (ETH is from issuer 2)
			OrbitRID:       "hrzid3",
			BaseAssetID:     btcAsset.ID,
			BaseAmount:      24.0,
			CounterAssetID:  ethAsset2.ID,
			CounterAmount:   26.0,
			Price:           0.92,
			LedgerCloseTime: now,
		},
		{ // BTC_ETH  trade (ETH is from issuer 1)
			OrbitRID:       "hrzid2",
			BaseAssetID:     btcAsset.ID,
			BaseAmount:      50.0,
			CounterAssetID:  ethAsset1.ID,
			CounterAmount:   50.0,
			Price:           1.0,
			LedgerCloseTime: oneHourAgo,
		},
		{ // BTC_ETH  trade (ETH is from issuer 1)
			OrbitRID:       "hrzid4",
			BaseAssetID:     btcAsset.ID,
			BaseAmount:      50.0,
			CounterAssetID:  ethAsset1.ID,
			CounterAmount:   6.0,
			Price:           0.12,
			LedgerCloseTime: threeDaysAgo,
		},
	}
	err = session.BulkInsertTrades(ctx, trades)
	require.NoError(t, err)

	// Adding some orderbook stats:
	obTime := time.Now()
	orderbookStats := OrderbookStats{
		BaseAssetID:    btcAsset.ID,
		CounterAssetID: ethAsset1.ID,
		NumBids:        15,
		BidVolume:      0.15,
		HighestBid:     200.0,
		NumAsks:        17,
		AskVolume:      30.0,
		LowestAsk:      0.1,
		Spread:         0.93,
		SpreadMidPoint: 0.35,
		UpdatedAt:      obTime,
	}
	err = session.InsertOrUpdateOrderbookStats(ctx,
		&orderbookStats,
		[]string{"base_asset_id", "counter_asset_id"},
	)
	require.NoError(t, err)

	var obBTCETH1 OrderbookStats
	err = session.GetRaw(ctx, &obBTCETH1, `
		SELECT *
		FROM orderbook_stats
		ORDER BY id DESC
		LIMIT 1`,
	)
	require.NoError(t, err)

	orderbookStats = OrderbookStats{
		BaseAssetID:    btcAsset.ID,
		CounterAssetID: ethAsset2.ID,
		NumBids:        1,
		BidVolume:      0.1,
		HighestBid:     20.0,
		NumAsks:        1,
		AskVolume:      15.0,
		LowestAsk:      0.2,
		Spread:         0.96,
		SpreadMidPoint: 0.36,
		UpdatedAt:      obTime,
	}
	err = session.InsertOrUpdateOrderbookStats(ctx,
		&orderbookStats,
		[]string{"base_asset_id", "counter_asset_id"},
	)
	require.NoError(t, err)

	var obBTCETH2 OrderbookStats
	err = session.GetRaw(ctx, &obBTCETH2, `
		SELECT *
		FROM orderbook_stats
		ORDER BY id DESC
		LIMIT 1`,
	)
	require.NoError(t, err)
	assert.NotEqual(t, obBTCETH1.ID, obBTCETH2.ID)

	partialMkts, err := session.RetrievePartialMarkets(ctx,
		nil, nil, nil, nil, 12,
	)
	require.NoError(t, err)
	assert.Equal(t, 2, len(partialMkts))

	// Mapping the retrieved markets:
	var btceth1Mkt, btceth2Mkt PartialMarket
	for _, mkt := range partialMkts {
		if mkt.CounterAssetIssuer == issuer1PK {
			btceth1Mkt = mkt
		}

		if mkt.CounterAssetIssuer == issuer2PK {
			btceth2Mkt = mkt
		}
	}
	tradePair1 := fmt.Sprintf("BTC:%s / ETH:%s", issuer1PK, issuer1PK)
	tradePair2 := fmt.Sprintf("BTC:%s / ETH:%s", issuer1PK, issuer2PK)

	assert.Equal(t, tradePair1, btceth1Mkt.TradePairName)
	assert.Equal(t, tradePair2, btceth2Mkt.TradePairName)

	// Validating the aggregated data
	assert.Equal(t, 150.0, btceth1Mkt.BaseVolume)
	assert.Equal(t, 60.0, btceth1Mkt.CounterVolume)
	assert.Equal(t, int32(2), btceth1Mkt.TradeCount)
	assert.Equal(t, 1.0, btceth1Mkt.Open)
	assert.Equal(t, 0.1, btceth1Mkt.Close)
	assert.Equal(t, -0.9, btceth1Mkt.Change)
	assert.Equal(t, 1.0, btceth1Mkt.High)
	assert.Equal(t, 0.1, btceth1Mkt.Low)
	assert.WithinDuration(t, oneHourAgo.Local(), btceth1Mkt.FirstLedgerCloseTime.Local(), 10*time.Millisecond)
	assert.WithinDuration(t, tenMinutesAgo.Local(), btceth1Mkt.LastLedgerCloseTime.Local(), 10*time.Millisecond)
	assert.Equal(t, 24.0, btceth2Mkt.BaseVolume)
	assert.Equal(t, 26.0, btceth2Mkt.CounterVolume)
	assert.Equal(t, int32(1), btceth2Mkt.TradeCount)
	assert.Equal(t, 0.92, btceth2Mkt.Open)
	assert.Equal(t, 0.92, btceth2Mkt.Close)
	assert.Equal(t, 0.0, btceth2Mkt.Change)
	assert.Equal(t, 0.92, btceth2Mkt.High)
	assert.Equal(t, 0.92, btceth2Mkt.Low)
	assert.WithinDuration(t, now.Local(), btceth2Mkt.FirstLedgerCloseTime.Local(), 10*time.Millisecond)
	assert.WithinDuration(t, now.Local(), btceth2Mkt.LastLedgerCloseTime.Local(), 10*time.Millisecond)

	// Analyzing non-aggregated orderbook data
	assert.Equal(t, 15, btceth1Mkt.NumBids)
	assert.Equal(t, 0.15, btceth1Mkt.BidVolume)
	assert.Equal(t, 200.0, btceth1Mkt.HighestBid)
	assert.Equal(t, 17, btceth1Mkt.NumAsks)
	assert.Equal(t, 30.0, btceth1Mkt.AskVolume)
	assert.Equal(t, 0.1, btceth1Mkt.LowestAsk)

	assert.Equal(t, 1, btceth2Mkt.NumBids)
	assert.Equal(t, 0.1, btceth2Mkt.BidVolume)
	assert.Equal(t, 20.0, btceth2Mkt.HighestBid)
	assert.Equal(t, 1, btceth2Mkt.NumAsks)
	assert.Equal(t, 15.0, btceth2Mkt.AskVolume)
	assert.Equal(t, 0.2, btceth2Mkt.LowestAsk)

	// Now let's use the same data, but aggregating by asset pair
	partialAggMkts, err := session.RetrievePartialAggMarkets(ctx, nil, 12)
	require.NoError(t, err)
	assert.Equal(t, 1, len(partialAggMkts))

	partialAggMkt := partialAggMkts[0]

	assert.Equal(t, "BTC_ETH", partialAggMkt.TradePairName)
	assert.Equal(t, 174.0, partialAggMkt.BaseVolume)
	assert.Equal(t, 86.0, partialAggMkt.CounterVolume)
	assert.Equal(t, int32(3), partialAggMkt.TradeCount)
	assert.Equal(t, 1.0, partialAggMkt.Open)
	assert.Equal(t, 0.92, partialAggMkt.Close)
	assert.Equal(t, 1.0, partialAggMkt.High)
	assert.Equal(t, 0.1, partialAggMkt.Low)
	assert.WithinDuration(t, oneHourAgo.Local(), partialAggMkt.FirstLedgerCloseTime.Local(), 10*time.Millisecond)
	assert.WithinDuration(t, now.Local(), partialAggMkt.LastLedgerCloseTime.Local(), 10*time.Millisecond)

	// There might be some floating point rounding issues, so this test
	// needs to be a bit more flexible. Since the change is 0.08, an error
	// around 0.0000000000001 is acceptable:
	priceDiff := math.Abs(-0.08 - partialAggMkt.Change)
	assert.True(t, priceDiff < 0.0000000000001)

	// Validate the pair name parsing:
	pairName := new(string)
	*pairName = "BTC_ETH"
	partialAggMkts, err = session.RetrievePartialAggMarkets(ctx, pairName, 12)
	require.NoError(t, err)
	assert.Equal(t, 1, len(partialAggMkts))
	assert.Equal(t, int32(3), partialAggMkts[0].TradeCount)

	// Analyzing aggregated orderbook data:
	assert.Equal(t, 16, partialAggMkt.NumBids)
	assert.Equal(t, 0.25, partialAggMkt.BidVolume)
	assert.Equal(t, 200.0, partialAggMkt.HighestBid)
	assert.Equal(t, 18, partialAggMkt.NumAsks)
	assert.Equal(t, 45.0, partialAggMkt.AskVolume)
	assert.Equal(t, 0.1, partialAggMkt.LowestAsk)
}

func Test24hStatsFallback(t *testing.T) {
	db := OpenTestDBConnection(t)
	defer db.Close()

	var session TickerSession
	session.DB = db.Open()
	ctx := context.Background()
	defer session.DB.Close()

	// Run migrations to make sure the tests are run
	// on the most updated schema version
	migrations := &migrate.FileMigrationSource{
		Dir: "./migrations",
	}
	_, err := migrate.Exec(session.DB.DB, "postgres", migrations, migrate.Up)
	require.NoError(t, err)

	// Adding a seed issuer to be used later:
	tbl := session.GetTable("issuers")
	_, err = tbl.Insert(Issuer{
		PublicKey: "GCF3TQXKZJNFJK7HCMNE2O2CUNKCJH2Y2ROISTBPLC7C5EIA5NNG2XZB",
		Name:      "FOO BAR",
	}).IgnoreCols("id").Exec(ctx)
	require.NoError(t, err)
	var issuer Issuer
	err = session.GetRaw(ctx, &issuer, `
		SELECT *
		FROM issuers
		ORDER BY id DESC
		LIMIT 1`,
	)
	require.NoError(t, err)

	// Adding a seed asset to be used later:
	err = session.InsertOrUpdateAsset(ctx, &Asset{
		Code:     "GRAM",
		IssuerID: issuer.ID,
		IsValid:  true,
	}, []string{"code", "issuer_id"})
	require.NoError(t, err)
	var gramAsset Asset
	err = session.GetRaw(ctx, &gramAsset, `
		SELECT *
		FROM assets
		ORDER BY id DESC
		LIMIT 1`,
	)
	require.NoError(t, err)

	// Adding another asset to be used later:
	err = session.InsertOrUpdateAsset(ctx, &Asset{
		Code:     "BTC",
		IssuerID: issuer.ID,
		IsValid:  true,
	}, []string{"code", "issuer_id"})
	require.NoError(t, err)
	var btcAsset Asset
	err = session.GetRaw(ctx, &btcAsset, `
		SELECT *
		FROM assets
		ORDER BY id DESC
		LIMIT 1`,
	)
	require.NoError(t, err)

	// A few times to be used:
	now := time.Now()
	twoDaysAgo := now.AddDate(0, 0, -3)
	threeDaysAgo := now.AddDate(0, 0, -3)

	// Now let's create the trades:
	trades := []Trade{
		{
			OrbitRID:       "hrzid1",
			BaseAssetID:     gramAsset.ID,
			BaseAmount:      1.0,
			CounterAssetID:  btcAsset.ID,
			CounterAmount:   1.0,
			Price:           0.5, // close price & lowest price
			LedgerCloseTime: twoDaysAgo,
		},
		{ // BTC_ETH trade (ETH is from issuer 2)
			OrbitRID:       "hrzid2",
			BaseAssetID:     gramAsset.ID,
			BaseAmount:      1.0,
			CounterAssetID:  btcAsset.ID,
			CounterAmount:   1.0,
			Price:           1.0, // open price & highest price
			LedgerCloseTime: threeDaysAgo,
		},
	}
	err = session.BulkInsertTrades(ctx, trades)
	require.NoError(t, err)

	markets, err := session.RetrieveMarketData(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, len(markets))
	mkt := markets[0]

	// When there are no 24h data, 24h OHLC should fallback to the 7d close value
	assert.Equal(t, 0.5, mkt.LastPrice)
	assert.Equal(t, 0.5, mkt.LowestPrice24h)
	assert.Equal(t, 0.5, mkt.OpenPrice24h)
	assert.Equal(t, 0.5, mkt.HighestPrice24h)
}

func TestPreferAnchorAssetCode(t *testing.T) {
	db := OpenTestDBConnection(t)
	defer db.Close()

	var session TickerSession
	session.DB = db.Open()
	ctx := context.Background()
	defer session.DB.Close()

	// Run migrations to make sure the tests are run
	// on the most updated schema version
	migrations := &migrate.FileMigrationSource{
		Dir: "./migrations",
	}
	_, err := migrate.Exec(session.DB.DB, "postgres", migrations, migrate.Up)
	require.NoError(t, err)

	// Adding a seed issuer to be used later:
	tbl := session.GetTable("issuers")
	_, err = tbl.Insert(Issuer{
		PublicKey: "GCF3TQXKZJNFJK7HCMNE2O2CUNKCJH2Y2ROISTBPLC7C5EIA5NNG2XZB",
		Name:      "FOO BAR",
	}).IgnoreCols("id").Exec(ctx)
	require.NoError(t, err)
	var issuer Issuer
	err = session.GetRaw(ctx, &issuer, `
		SELECT *
		FROM issuers
		ORDER BY id DESC
		LIMIT 1`,
	)
	require.NoError(t, err)

	// Adding a seed asset to be used later:
	err = session.InsertOrUpdateAsset(ctx, &Asset{
		Code:     "GRAM",
		IssuerID: issuer.ID,
		IsValid:  true,
	}, []string{"code", "issuer_id"})
	require.NoError(t, err)
	var gramAsset Asset
	err = session.GetRaw(ctx, &gramAsset, `
		SELECT *
		FROM assets
		ORDER BY id DESC
		LIMIT 1`,
	)
	require.NoError(t, err)

	// Adding another asset to be used later:
	err = session.InsertOrUpdateAsset(ctx, &Asset{
		Code:            "EURT",
		IssuerID:        issuer.ID,
		IsValid:         true,
		AnchorAssetCode: "EUR",
	}, []string{"code", "issuer_id"})
	require.NoError(t, err)
	var btcAsset Asset
	err = session.GetRaw(ctx, &btcAsset, `
		SELECT *
		FROM assets
		ORDER BY id DESC
		LIMIT 1`,
	)
	require.NoError(t, err)

	// A few times to be used:
	now := time.Now()
	twoDaysAgo := now.AddDate(0, 0, -3)
	threeDaysAgo := now.AddDate(0, 0, -3)

	// Now let's create the trades:
	trades := []Trade{
		{
			OrbitRID:       "hrzid1",
			BaseAssetID:     gramAsset.ID,
			BaseAmount:      1.0,
			CounterAssetID:  btcAsset.ID,
			CounterAmount:   1.0,
			Price:           0.5, // close price & lowest price
			LedgerCloseTime: twoDaysAgo,
		},
		{ // BTC_ETH trade (ETH is from issuer 2)
			OrbitRID:       "hrzid2",
			BaseAssetID:     gramAsset.ID,
			BaseAmount:      1.0,
			CounterAssetID:  btcAsset.ID,
			CounterAmount:   1.0,
			Price:           1.0, // open price & highest price
			LedgerCloseTime: threeDaysAgo,
		},
	}
	err = session.BulkInsertTrades(ctx, trades)
	require.NoError(t, err)

	markets, err := session.RetrieveMarketData(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, len(markets))
	for _, mkt := range markets {
		require.Equal(t, "GRAM_EUR", mkt.TradePair)
	}

	partialAggMkts, err := session.RetrievePartialAggMarkets(ctx, nil, 168)
	require.NoError(t, err)
	assert.Equal(t, 1, len(partialAggMkts))
	for _, aggMkt := range partialAggMkts {
		require.Equal(t, "GRAM_EUR", aggMkt.TradePairName)
	}
}
