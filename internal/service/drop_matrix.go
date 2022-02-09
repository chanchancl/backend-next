package service

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ahmetb/go-linq/v3"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
	"gopkg.in/guregu/null.v3"

	"github.com/penguin-statistics/backend-next/internal/models"
	"github.com/penguin-statistics/backend-next/internal/utils"
)

type DropMatrixService struct {
	TimeRangeService         *TimeRangeService
	DropReportService        *DropReportService
	DropInfoService          *DropInfoService
	DropMatrixElementService *DropMatrixElementService
}

func NewDropMatrixService(
	timeRangeService *TimeRangeService,
	dropReportService *DropReportService,
	dropInfoService *DropInfoService,
	dropMatrixElementService *DropMatrixElementService,
) *DropMatrixService {
	return &DropMatrixService{
		TimeRangeService:         timeRangeService,
		DropReportService:        dropReportService,
		DropInfoService:          dropInfoService,
		DropMatrixElementService: dropMatrixElementService,
	}
}

func (s *DropMatrixService) GetSavedDropMatrixResults(ctx *fiber.Ctx, server string, accountId *null.Int) (*models.DropMatrixQueryResult, error) {
	dropMatrixElements, err := s.getDropMatrixElements(ctx, server, accountId)
	if err != nil {
		return nil, err
	}
	return s.convertDropMatrixElementsToMaxAccumulableDropMatrixQueryResult(ctx, server, dropMatrixElements)
}

func (s *DropMatrixService) QueryDropMatrix(
	ctx *fiber.Ctx, server string, timeRanges []*models.TimeRange, stageIdFilter []int, itemIdFilter []int, accountId *null.Int,
) (*models.DropMatrixQueryResult, error) {
	dropMatrixElements, err := s.calcDropMatrixForTimeRanges(ctx, server, timeRanges, stageIdFilter, itemIdFilter, accountId)
	if err != nil {
		return nil, err
	}
	return s.convertDropMatrixElementsToDropMatrixQueryResult(ctx, server, dropMatrixElements)
}

func (s *DropMatrixService) RefreshAllDropMatrixElements(ctx *fiber.Ctx, server string) error {
	toSave := []*models.DropMatrixElement{}
	allTimeRanges, err := s.TimeRangeService.GetAllTimeRangesByServer(ctx, server)
	if err != nil {
		return err
	}
	ch := make(chan []*models.DropMatrixElement, 15)
	var wg sync.WaitGroup

	go func() {
		for {
			m := <-ch
			toSave = append(toSave, m...)
			wg.Done()
		}
	}()

	usedTimeMap := sync.Map{}

	limiter := make(chan struct{}, 7)
	wg.Add(len(allTimeRanges))
	for _, timeRange := range allTimeRanges {
		limiter <- struct{}{}
		go func(timeRange *models.TimeRange) {
			fmt.Println("<   :", timeRange.RangeID)
			startTime := time.Now()

			timeRanges := []*models.TimeRange{timeRange}
			currentBatch, err := s.calcDropMatrixForTimeRanges(ctx, server, timeRanges, nil, nil, &null.Int{})
			if err != nil {
				return
			}

			ch <- currentBatch
			<-limiter

			usedTimeMap.Store(timeRange.RangeID, int(time.Since(startTime).Microseconds()))
			fmt.Println("   > :", timeRange.RangeID, "@", time.Since(startTime))
		}(timeRange)
	}

	wg.Wait()

	log.Debug().Msgf("toSave length: %v", len(toSave))
	return s.DropMatrixElementService.BatchSaveElements(ctx, toSave, server)
}

func (s *DropMatrixService) getDropMatrixElements(ctx *fiber.Ctx, server string, accountId *null.Int) ([]*models.DropMatrixElement, error) {
	if accountId.Valid {
		maxAccumulableTimeRanges, err := s.TimeRangeService.GetMaxAccumulableTimeRangesByServer(ctx, server)
		if err != nil {
			return nil, err
		}
		timeRanges := make([]*models.TimeRange, 0)

		timeRangesMap := make(map[int]*models.TimeRange, 0)
		for _, maxAccumulableTimeRangesForOneStage := range maxAccumulableTimeRanges {
			for _, timeRanges := range maxAccumulableTimeRangesForOneStage {
				for _, timeRange := range timeRanges {
					timeRangesMap[timeRange.RangeID] = timeRange
				}
			}
		}
		for _, timeRange := range timeRangesMap {
			timeRanges = append(timeRanges, timeRange)
		}
		return s.calcDropMatrixForTimeRanges(ctx, server, timeRanges, nil, nil, accountId)
	} else {
		return s.DropMatrixElementService.GetElementsByServer(ctx, server)
	}
}

func (s *DropMatrixService) calcDropMatrixForTimeRanges(
	ctx *fiber.Ctx, server string, timeRanges []*models.TimeRange, stageIdFilter []int, itemIdFilter []int, accountId *null.Int,
) ([]*models.DropMatrixElement, error) {
	dropInfos, err := s.DropInfoService.GetDropInfosWithFilters(ctx, server, timeRanges, stageIdFilter, itemIdFilter)
	if err != nil {
		return nil, err
	}

	var combinedResults []*models.CombinedResultForDropMatrix
	for _, timeRange := range timeRanges {
		quantityResults, err := s.DropReportService.CalcTotalQuantityForDropMatrix(ctx, server, timeRange, utils.GetStageIdItemIdMapFromDropInfos(dropInfos), accountId)
		if err != nil {
			return nil, err
		}
		timesResults, err := s.DropReportService.CalcTotalTimesForDropMatrix(ctx, server, timeRange, utils.GetStageIdsFromDropInfos(dropInfos), accountId)
		if err != nil {
			return nil, err
		}
		combinedResults = s.combineQuantityAndTimesResults(quantityResults, timesResults, timeRange)
	}

	// save stage times for later use
	stageTimesMap := map[int]int{}

	// grouping results by stage id
	var groupedResults []linq.Group
	linq.From(combinedResults).
		GroupByT(
			func(el *models.CombinedResultForDropMatrix) int { return el.StageID },
			func(el *models.CombinedResultForDropMatrix) *models.CombinedResultForDropMatrix { return el }).ToSlice(&groupedResults)

	dropMatrixElements := make([]*models.DropMatrixElement, 0)
	for _, el := range groupedResults {
		stageId := el.Key.(int)
		var groupedResults2 []linq.Group
		linq.From(el.Group).
			GroupByT(
				func(el *models.CombinedResultForDropMatrix) int { return el.TimeRange.RangeID },
				func(el *models.CombinedResultForDropMatrix) *models.CombinedResultForDropMatrix { return el }).
			ToSlice(&groupedResults2)
		for _, el2 := range groupedResults2 {
			rangeId := el2.Key.(int)
			timeRange := el2.Group[0].(*models.CombinedResultForDropMatrix).TimeRange

			// get all item ids which are dropped in this stage and in this time range
			var dropItemIds []int
			if rangeId == 0 {
				// rangeId == 0 means it is a customized time range instead of a time range from the database
				dropInfosForSpecialTimeRange, err := s.DropInfoService.GetDropInfosWithFilters(ctx, server, []*models.TimeRange{el2.Group[0].(*models.CombinedResultForDropMatrix).TimeRange}, nil, nil)
				if err != nil {
					return nil, err
				}
				linq.From(dropInfosForSpecialTimeRange).
					WhereT(func(el *models.DropInfo) bool { return el.ItemID.Valid }).
					SelectT(func(el *models.DropInfo) int { return int(el.ItemID.Int64) }).
					ToSlice(&dropItemIds)
			} else {
				dropItemIds, _ = s.DropInfoService.GetItemDropSetByStageIdAndRangeId(ctx, server, stageId, rangeId)
			}

			// if item id filter is applied, then filter the drop item ids
			if len(itemIdFilter) > 0 {
				linq.From(dropItemIds).WhereT(func(itemId int) bool { return linq.From(itemIdFilter).Contains(itemId) }).ToSlice(&dropItemIds)
			}

			// use a fake hashset to save item ids
			dropSet := make(map[int]struct{}, len(dropItemIds))
			for _, itemId := range dropItemIds {
				dropSet[itemId] = struct{}{}
			}

			for _, el3 := range el2.Group {
				itemId := el3.(*models.CombinedResultForDropMatrix).ItemID
				quantity := el3.(*models.CombinedResultForDropMatrix).Quantity
				times := el3.(*models.CombinedResultForDropMatrix).Times
				dropMatrixElement := models.DropMatrixElement{
					StageID:  stageId,
					ItemID:   itemId,
					RangeID:  rangeId,
					Quantity: quantity,
					Times:    times,
					Server:   server,
				}
				if rangeId == 0 {
					dropMatrixElement.TimeRange = timeRange
				}
				dropMatrixElements = append(dropMatrixElements, &dropMatrixElement)
				delete(dropSet, itemId)        // remove existing item ids from drop set
				stageTimesMap[stageId] = times // record stage times into a map
			}
			// add those items which do not show up in the matrix (quantity is 0)
			for itemId := range dropSet {
				dropMatrixElementWithZeroQuantity := models.DropMatrixElement{
					StageID:  stageId,
					ItemID:   itemId,
					RangeID:  rangeId,
					Quantity: 0,
					Times:    stageTimesMap[stageId],
					Server:   server,
				}
				if rangeId == 0 {
					dropMatrixElementWithZeroQuantity.TimeRange = timeRange
				}
				dropMatrixElements = append(dropMatrixElements, &dropMatrixElementWithZeroQuantity)
			}
		}
	}
	return dropMatrixElements, nil
}

func (s *DropMatrixService) combineQuantityAndTimesResults(
	quantityResults []*models.TotalQuantityResultForDropMatrix, timesResults []*models.TotalTimesResult, timeRange *models.TimeRange,
) []*models.CombinedResultForDropMatrix {
	var firstGroupResults []linq.Group
	combinedResults := make([]*models.CombinedResultForDropMatrix, 0)
	linq.From(quantityResults).
		GroupByT(
			func(result *models.TotalQuantityResultForDropMatrix) int { return result.StageID },
			func(result *models.TotalQuantityResultForDropMatrix) *models.TotalQuantityResultForDropMatrix {
				return result
			}).
		ToSlice(&firstGroupResults)
	quantityResultsMap := make(map[int]map[int]int)
	for _, firstGroupElements := range firstGroupResults {
		stageId := firstGroupElements.Key.(int)
		resultsMap := make(map[int]int, 0)
		linq.From(firstGroupElements.Group).
			ToMapByT(&resultsMap,
				func(el interface{}) int { return el.(*models.TotalQuantityResultForDropMatrix).ItemID },
				func(el interface{}) int { return el.(*models.TotalQuantityResultForDropMatrix).TotalQuantity })
		quantityResultsMap[stageId] = resultsMap
	}

	var secondGroupResults []linq.Group
	linq.From(timesResults).
		GroupByT(
			func(result *models.TotalTimesResult) int { return result.StageID },
			func(result *models.TotalTimesResult) *models.TotalTimesResult { return result }).
		ToSlice(&secondGroupResults)
	for _, secondGroupResults := range secondGroupResults {
		stageId := secondGroupResults.Key.(int)
		quantityResultsMapForOneStage := quantityResultsMap[stageId]
		for _, el := range secondGroupResults.Group {
			times := el.(*models.TotalTimesResult).TotalTimes
			for itemId, quantity := range quantityResultsMapForOneStage {
				combinedResults = append(combinedResults, &models.CombinedResultForDropMatrix{
					StageID:   stageId,
					ItemID:    itemId,
					Quantity:  quantity,
					Times:     times,
					TimeRange: timeRange,
				})
			}
		}
	}
	return combinedResults
}

func (s *DropMatrixService) convertDropMatrixElementsToMaxAccumulableDropMatrixQueryResult(
	ctx *fiber.Ctx, server string, dropMatrixElements []*models.DropMatrixElement,
) (*models.DropMatrixQueryResult, error) {
	elementsMap := utils.GetDropMatrixElementsMap(dropMatrixElements)
	result := &models.DropMatrixQueryResult{
		Matrix: make([]*models.OneDropMatrixElement, 0),
	}

	maxAccumulableTimeRanges, err := s.TimeRangeService.GetMaxAccumulableTimeRangesByServer(ctx, server)
	if err != nil {
		return nil, err
	}
	for stageId, maxAccumulableTimeRangesForOneStage := range maxAccumulableTimeRanges {
		subMapByItemId := elementsMap[stageId]
		for itemId, timeRanges := range maxAccumulableTimeRangesForOneStage {
			subMapByRangeId := subMapByItemId[itemId]
			startTime := timeRanges[0].StartTime
			endTime := timeRanges[0].EndTime
			var combinedDropMatrixResult *models.OneDropMatrixElement
			combinedDropMatrixResult = nil
			for _, timeRange := range timeRanges {
				element, ok := subMapByRangeId[timeRange.RangeID]
				if !ok {
					continue
				}
				oneElementResult := &models.OneDropMatrixElement{
					StageID:  stageId,
					ItemID:   itemId,
					Quantity: element.Quantity,
					Times:    element.Times,
				}
				if timeRange.StartTime.Before(*startTime) {
					startTime = timeRange.StartTime
				}
				if timeRange.EndTime.After(*endTime) {
					endTime = timeRange.EndTime
				}
				if combinedDropMatrixResult == nil {
					combinedDropMatrixResult = oneElementResult
				} else {
					combinedDropMatrixResult, err = s.combineDropMatrixResults(combinedDropMatrixResult, oneElementResult)
					if err != nil {
						return nil, err
					}
				}
			}
			if combinedDropMatrixResult != nil {
				combinedDropMatrixResult.TimeRange = &models.TimeRange{
					StartTime: startTime,
					EndTime:   endTime,
				}
				result.Matrix = append(result.Matrix, combinedDropMatrixResult)
			}
		}
	}
	return result, nil
}

func (s *DropMatrixService) combineDropMatrixResults(a *models.OneDropMatrixElement, b *models.OneDropMatrixElement) (*models.OneDropMatrixElement, error) {
	if a.StageID != b.StageID {
		return nil, errors.New("stageId not match")
	}
	if a.ItemID != b.ItemID {
		return nil, errors.New("itemId not match")
	}
	result := &models.OneDropMatrixElement{
		StageID:  a.StageID,
		ItemID:   a.ItemID,
		Quantity: a.Quantity + b.Quantity,
		Times:    a.Times + b.Times,
	}
	return result, nil
}

func (s *DropMatrixService) convertDropMatrixElementsToDropMatrixQueryResult(ctx *fiber.Ctx, server string, dropMatrixElements []*models.DropMatrixElement) (*models.DropMatrixQueryResult, error) {
	dropMatrixQueryResult := &models.DropMatrixQueryResult{
		Matrix: make([]*models.OneDropMatrixElement, 0),
	}
	var groupedResults []linq.Group
	linq.From(dropMatrixElements).
		GroupByT(
			func(el *models.DropMatrixElement) int { return el.RangeID },
			func(el *models.DropMatrixElement) *models.DropMatrixElement { return el },
		).
		ToSlice(&groupedResults)
	for _, group := range groupedResults {
		rangeId := group.Key.(int)
		var timeRange *models.TimeRange
		if rangeId == 0 {
			timeRange = group.Group[0].(*models.DropMatrixElement).TimeRange
		} else {
			tr, err := s.TimeRangeService.GetTimeRangeById(ctx, rangeId)
			if err != nil {
				return nil, err
			}
			timeRange = tr
		}

		for _, el := range group.Group {
			dropMatrixElement := el.(*models.DropMatrixElement)
			dropMatrixQueryResult.Matrix = append(dropMatrixQueryResult.Matrix, &models.OneDropMatrixElement{
				StageID:   dropMatrixElement.StageID,
				ItemID:    dropMatrixElement.ItemID,
				Quantity:  dropMatrixElement.Quantity,
				Times:     dropMatrixElement.Times,
				TimeRange: timeRange,
			})
		}
	}
	return dropMatrixQueryResult, nil
}
