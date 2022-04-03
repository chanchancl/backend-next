package cache

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"gopkg.in/guregu/null.v3"

	"github.com/penguin-statistics/backend-next/internal/models"
	modelsv2 "github.com/penguin-statistics/backend-next/internal/models/v2"
	"github.com/penguin-statistics/backend-next/internal/pkg/cache"
	"github.com/penguin-statistics/backend-next/internal/repos"
)

type Flusher func() error

var (
	AccountByID        *cache.Set[models.Account]
	AccountByPenguinID *cache.Set[models.Account]

	ItemDropSetByStageIDAndRangeID   *cache.Set[[]int]
	ItemDropSetByStageIdAndTimeRange *cache.Set[[]int]

	ShimMaxAccumulableDropMatrixResults *cache.Set[modelsv2.DropMatrixQueryResult]

	Formula *cache.Singular[json.RawMessage]

	Items           *cache.Singular[[]*models.Item]
	ItemByArkID     *cache.Set[models.Item]
	ShimItems       *cache.Singular[[]*modelsv2.Item]
	ShimItemByArkID *cache.Set[modelsv2.Item]
	ItemsMapById    *cache.Singular[map[int]*models.Item]
	ItemsMapByArkID *cache.Singular[map[string]*models.Item]

	Notices *cache.Singular[[]*models.Notice]

	Activities     *cache.Singular[[]*models.Activity]
	ShimActivities *cache.Singular[[]*modelsv2.Activity]

	ShimLatestPatternMatrixResults *cache.Set[modelsv2.PatternMatrixQueryResult]

	ShimSiteStats *cache.Set[modelsv2.SiteStats]

	Stages           *cache.Singular[[]*models.Stage]
	StageByArkID     *cache.Set[models.Stage]
	ShimStages       *cache.Set[[]*modelsv2.Stage]
	ShimStageByArkID *cache.Set[modelsv2.Stage]
	StagesMapByID    *cache.Singular[map[int]*models.Stage]
	StagesMapByArkID *cache.Singular[map[string]*models.Stage]

	TimeRanges               *cache.Set[[]*models.TimeRange]
	TimeRangeByID            *cache.Set[models.TimeRange]
	TimeRangesMap            *cache.Set[map[int]*models.TimeRange]
	MaxAccumulableTimeRanges *cache.Set[map[int]map[int][]*models.TimeRange]

	ShimSavedTrendResults *cache.Set[modelsv2.TrendQueryResult]

	Zones           *cache.Singular[[]*models.Zone]
	ZoneByArkID     *cache.Set[models.Zone]
	ShimZones       *cache.Singular[[]*modelsv2.Zone]
	ShimZoneByArkID *cache.Set[modelsv2.Zone]

	DropPatternElementsByPatternID *cache.Set[[]*models.DropPatternElement]

	LastModifiedTime *cache.Set[time.Time]

	Properties map[string]string

	once sync.Once

	CacheSetMap             map[string]Flusher
	CacheSingularFlusherMap map[string]Flusher
)

func Initialize(propertyRepo *repos.PropertyRepo) {
	once.Do(func() {
		initializeCaches()
		populateProperties(propertyRepo)
	})
}

func Delete(name string, key null.String) error {
	if key.Valid {
		if _, ok := CacheSetMap[name]; ok {
			if err := CacheSetMap[name](); err != nil {
				return err
			}
		}
	} else {
		if _, ok := CacheSingularFlusherMap[name]; ok {
			if err := CacheSingularFlusherMap[name](); err != nil {
				return err
			}
		} else if _, ok := CacheSetMap[name]; ok {
			if err := CacheSetMap[name](); err != nil {
				return err
			}
		}
	}
	return nil
}

func initializeCaches() {
	CacheSetMap = make(map[string]Flusher)
	CacheSingularFlusherMap = make(map[string]Flusher)

	// account
	AccountByID = cache.NewSet[models.Account]("account#accountId")
	AccountByPenguinID = cache.NewSet[models.Account]("account#penguinId")

	CacheSetMap["account#accountId"] = AccountByID.Flush
	CacheSetMap["account#penguinId"] = AccountByPenguinID.Flush

	// drop_info
	ItemDropSetByStageIDAndRangeID = cache.NewSet[[]int]("itemDropSet#server|stageId|rangeId")
	ItemDropSetByStageIdAndTimeRange = cache.NewSet[[]int]("itemDropSet#server|stageId|startTime|endTime")

	CacheSetMap["itemDropSet#server|stageId|rangeId"] = ItemDropSetByStageIDAndRangeID.Flush
	CacheSetMap["itemDropSet#server|stageId|startTime|endTime"] = ItemDropSetByStageIdAndTimeRange.Flush

	// drop_matrix
	ShimMaxAccumulableDropMatrixResults = cache.NewSet[modelsv2.DropMatrixQueryResult]("shimMaxAccumulableDropMatrixResults#server|showClosedZoned")

	CacheSetMap["shimMaxAccumulableDropMatrixResults#server|showClosedZoned"] = ShimMaxAccumulableDropMatrixResults.Flush

	// formula
	Formula = cache.NewSingular[json.RawMessage]("formula")
	CacheSingularFlusherMap["formula"] = Formula.Delete

	// item
	Items = cache.NewSingular[[]*models.Item]("items")
	ItemByArkID = cache.NewSet[models.Item]("item#arkItemId")
	ShimItems = cache.NewSingular[[]*modelsv2.Item]("shimItems")
	ShimItemByArkID = cache.NewSet[modelsv2.Item]("shimItem#arkItemId")
	ItemsMapById = cache.NewSingular[map[int]*models.Item]("itemsMapById")
	ItemsMapByArkID = cache.NewSingular[map[string]*models.Item]("itemsMapByArkId")

	CacheSingularFlusherMap["items"] = Items.Delete
	CacheSetMap["item#arkItemId"] = ItemByArkID.Flush
	CacheSingularFlusherMap["shimItems"] = ShimItems.Delete
	CacheSetMap["shimItem#arkItemId"] = ShimItemByArkID.Flush
	CacheSingularFlusherMap["itemsMapById"] = ItemsMapById.Delete
	CacheSingularFlusherMap["itemsMapByArkId"] = ItemsMapByArkID.Delete

	// notice
	Notices = cache.NewSingular[[]*models.Notice]("notices")

	CacheSingularFlusherMap["notices"] = Notices.Delete

	// activity
	Activities = cache.NewSingular[[]*models.Activity]("activities")
	ShimActivities = cache.NewSingular[[]*modelsv2.Activity]("shimActivities")

	CacheSingularFlusherMap["activities"] = Activities.Delete
	CacheSingularFlusherMap["shimActivities"] = ShimActivities.Delete

	// pattern_matrix
	ShimLatestPatternMatrixResults = cache.NewSet[modelsv2.PatternMatrixQueryResult]("shimLatestPatternMatrixResults#server")

	CacheSetMap["shimLatestPatternMatrixResults#server"] = ShimLatestPatternMatrixResults.Flush

	// site_stats
	ShimSiteStats = cache.NewSet[modelsv2.SiteStats]("shimSiteStats#server")

	CacheSetMap["shimSiteStats#server"] = ShimSiteStats.Flush

	// stage
	Stages = cache.NewSingular[[]*models.Stage]("stages")
	StageByArkID = cache.NewSet[models.Stage]("stage#arkStageId")
	ShimStages = cache.NewSet[[]*modelsv2.Stage]("shimStages#server")
	ShimStageByArkID = cache.NewSet[modelsv2.Stage]("shimStage#server|arkStageId")
	StagesMapByID = cache.NewSingular[map[int]*models.Stage]("stagesMapById")
	StagesMapByArkID = cache.NewSingular[map[string]*models.Stage]("stagesMapByArkId")

	CacheSingularFlusherMap["stages"] = Stages.Delete
	CacheSetMap["stage#arkStageId"] = StageByArkID.Flush
	CacheSetMap["shimStages#server"] = ShimStages.Flush
	CacheSetMap["shimStage#server|arkStageId"] = ShimStageByArkID.Flush
	CacheSingularFlusherMap["stagesMapById"] = StagesMapByID.Delete
	CacheSingularFlusherMap["stagesMapByArkId"] = StagesMapByArkID.Delete

	// time_range
	TimeRanges = cache.NewSet[[]*models.TimeRange]("timeRanges#server")
	TimeRangeByID = cache.NewSet[models.TimeRange]("timeRange#rangeId")
	TimeRangesMap = cache.NewSet[map[int]*models.TimeRange]("timeRangesMap#server")
	MaxAccumulableTimeRanges = cache.NewSet[map[int]map[int][]*models.TimeRange]("maxAccumulableTimeRanges#server")

	CacheSetMap["timeRanges#server"] = TimeRanges.Flush
	CacheSetMap["timeRange#rangeId"] = TimeRangeByID.Flush
	CacheSetMap["timeRangesMap#server"] = TimeRangesMap.Flush
	CacheSetMap["maxAccumulableTimeRanges#server"] = MaxAccumulableTimeRanges.Flush

	// trend
	ShimSavedTrendResults = cache.NewSet[modelsv2.TrendQueryResult]("shimSavedTrendResults#server")

	CacheSetMap["shimSavedTrendResults#server"] = ShimSavedTrendResults.Flush

	// zone
	Zones = cache.NewSingular[[]*models.Zone]("zones")
	ZoneByArkID = cache.NewSet[models.Zone]("zone#arkZoneId")
	ShimZones = cache.NewSingular[[]*modelsv2.Zone]("shimZones")
	ShimZoneByArkID = cache.NewSet[modelsv2.Zone]("shimZone#arkZoneId")

	CacheSingularFlusherMap["zones"] = Zones.Delete
	CacheSetMap["zone#arkZoneId"] = ZoneByArkID.Flush
	CacheSingularFlusherMap["shimZones"] = ShimZones.Delete
	CacheSetMap["shimZone#arkZoneId"] = ShimZoneByArkID.Flush

	// drop_pattern_elements
	DropPatternElementsByPatternID = cache.NewSet[[]*models.DropPatternElement]("dropPatternElements#patternId")

	CacheSetMap["dropPatternElements#patternId"] = DropPatternElementsByPatternID.Flush

	// others
	LastModifiedTime = cache.NewSet[time.Time]("lastModifiedTime#key")

	CacheSetMap["lastModifiedTime#key"] = LastModifiedTime.Flush
}

func populateProperties(repo *repos.PropertyRepo) {
	Properties = make(map[string]string)
	properties, err := repo.GetProperties(context.Background())
	if err != nil {
		panic(err)
	}

	for _, property := range properties {
		Properties[property.Key] = property.Value
	}
}
