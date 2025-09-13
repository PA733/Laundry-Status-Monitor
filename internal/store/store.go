package store

import (
	"context"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"laundry-status-backend/internal/model"
	"laundry-status-backend/internal/parse"
)

// Store defines the interface for all database operations.
type Store interface {
	UpsertDormsAndMachines(ctx context.Context, items []ApiItem) error
	UpdateOccupancy(ctx context.Context, now time.Time, items []ApiItem, getStateType func(int) MachineStateType) error
}

// gormStore implements the Store interface using GORM.
type gormStore struct {
	db *gorm.DB
}

// NewGormStore creates a new GORM-backed store.
func NewGormStore(db *gorm.DB) Store {
	return &gormStore{db: db}
}

// UpdateOccupancy processes state changes and updates the database transactionally.
func (s *gormStore) UpdateOccupancy(ctx context.Context, now time.Time, allItems []ApiItem, getStateType func(int) MachineStateType) error {
	currentOpenRecords, err := s.fetchAllOpenOccupancies(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch open occupancy records: %w", err)
	}

	latestDataMap := make(map[int64]ApiItem)
	for _, item := range allItems {
		latestDataMap[item.ID] = item
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Process each machine from the latest API data.
		for _, machineData := range allItems {
			oldRecord, exists := currentOpenRecords[machineData.ID]

			if exists {
				// State has changed, archive the old record.
				if machineData.State != oldRecord.Status {
					if err := archiveRecord(tx, oldRecord, now); err != nil {
						return err
					}

					// 判断新状态
					if getStateType(machineData.State) == StateTypeIdle {
						// 如果新状态是 Idle，则从 open 表中删除该记录
						if err := tx.Delete(&model.OccupancyOpen{}, oldRecord.MachineID).Error; err != nil {
							return fmt.Errorf("failed to delete open occupancy record for machine %d: %w", oldRecord.MachineID, err)
						}
					} else {
						// 如果新状态不是 Idle，则更新记录
						updatedRecord := s.prepareOccupancy(machineData, now, getStateType)
						if err := tx.Save(&updatedRecord).Error; err != nil {
							return fmt.Errorf("failed to update occupancy record for machine %d: %w", machineData.ID, err)
						}
					}
				}
				// Remove the machine from the map to track which machines we've seen.
				delete(currentOpenRecords, machineData.ID)
			} else {
				// This is a new machine not previously tracked.
				if getStateType(machineData.State) != StateTypeIdle {
					newRecord := s.prepareOccupancy(machineData, now, getStateType)
					if err := tx.Create(&newRecord).Error; err != nil {
						return fmt.Errorf("failed to create new occupancy record for machine %d: %w", machineData.ID, err)
					}
				}
			}
		}

		// Handle machines that were in our database but are no longer in the API feed.
		for _, remainingRecord := range currentOpenRecords {
			if err := archiveRecord(tx, remainingRecord, now); err != nil {
				return err
			}
			if err := tx.Delete(&model.OccupancyOpen{}, remainingRecord.MachineID).Error; err != nil {
				return fmt.Errorf("failed to delete open occupancy record for machine %d: %w", remainingRecord.MachineID, err)
			}
		}
		return nil
	})
}

// archiveRecord creates a historical record of a completed machine state.
func archiveRecord(tx *gorm.DB, recordToArchive model.OccupancyOpen, observationTime time.Time) error {
	startTime := recordToArchive.ObservedAt
	// Calculate the PREDICTED end time.
	var periodEnd time.Time
	if recordToArchive.TimeRemaining > 0 {
		// Case A: 对于有预计时长的状态 (如 "使用中")
		// period 的结束时间 = 预计结束时间
		periodEnd = startTime.Add(time.Duration(recordToArchive.TimeRemaining) * time.Second)
	} else {
		// Case B: 对于无预计时长的状态 (如 "故障")
		// period 的结束时间 = 状态结束的观测时间
		periodEnd = observationTime
	}

	historyRecord := model.OccupancyHistory{
		MachineID:  recordToArchive.MachineID,
		ObservedAt: observationTime, // WHEN we confirmed the state's completion.
		Status:     recordToArchive.Status,
		Message:    recordToArchive.Message,
		// The 'period' field stores the PREDICTED time range.
		PeriodStart: startTime,
		PeriodEnd:   periodEnd,
	}

	if err := tx.Create(&historyRecord).Error; err != nil {
		return fmt.Errorf("failed to archive occupancy record for machine %d: %w", recordToArchive.MachineID, err)
	}
	return nil
}

// UpsertDormsAndMachines handles the database updates for dorm and machine metadata.
func (s *gormStore) UpsertDormsAndMachines(ctx context.Context, items []ApiItem) error {
	existingMachines, err := s.fetchAllMachines(ctx)
	if err != nil {
		log.Printf("Warning: could not pre-fetch machines: %v", err)
		existingMachines = make(map[int64]model.Machine)
	}

	// Phase 1: Process and save dorms
	dormMap, err := s.processAndSaveDorms(ctx, items)
	if err != nil {
		return fmt.Errorf("failed to process dorms: %w", err)
	}

	// Phase 2: Build machine slice for upserting
	var machinesToUpsert []model.Machine
	for _, item := range items {
		parsedName, err := parse.ParseName(item.Name, item.FloorCode)
		if err != nil {
			log.Printf("Error parsing name for item %d (%s): %v", item.ID, item.Name, err)
			continue
		}

		dorm, ok := dormMap[parsedName.Dorm]
		if !ok {
			log.Printf("Error: could not find dorm %q in map after upserting. Skipping machine %d.", parsedName.Dorm, item.ID)
			continue
		}

		machine, needsUpsert := prepareMachine(item, parsedName, existingMachines, dorm.ID)
		if needsUpsert {
			machinesToUpsert = append(machinesToUpsert, machine)
		}
	}

	// Execute batch operation for machines
	if len(machinesToUpsert) > 0 {
		log.Printf("Batch upserting %d machines...", len(machinesToUpsert))
		return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return batchUpsertMachines(tx, machinesToUpsert)
		})
	}
	return nil
}

// --- Helper functions moved from scraper ---

func (s *gormStore) fetchAllOpenOccupancies(ctx context.Context) (map[int64]model.OccupancyOpen, error) {
	var openRecords []model.OccupancyOpen
	if err := s.db.WithContext(ctx).Find(&openRecords).Error; err != nil {
		return nil, err
	}
	recordMap := make(map[int64]model.OccupancyOpen, len(openRecords))
	for _, r := range openRecords {
		recordMap[r.MachineID] = r
	}
	return recordMap, nil
}

func (s *gormStore) fetchAllMachines(ctx context.Context) (map[int64]model.Machine, error) {
	var machines []model.Machine
	if err := s.db.WithContext(ctx).Find(&machines).Error; err != nil {
		return nil, err
	}
	machineMap := make(map[int64]model.Machine, len(machines))
	for _, m := range machines {
		machineMap[m.ID] = m
	}
	return machineMap, nil
}

func (s *gormStore) processAndSaveDorms(ctx context.Context, items []ApiItem) (map[string]model.Dorm, error) {
	dormsToUpsert := make(map[string]model.Dorm)
	for _, item := range items {
		parsedName, err := parse.ParseName(item.Name, item.FloorCode)
		if err != nil {
			continue
		}
		if _, exists := dormsToUpsert[parsedName.Dorm]; !exists {
			dormsToUpsert[parsedName.Dorm] = model.Dorm{Name: parsedName.Dorm}
		}
	}

	if len(dormsToUpsert) == 0 {
		return make(map[string]model.Dorm), nil
	}

	var dormList []model.Dorm
	for _, d := range dormsToUpsert {
		dormList = append(dormList, d)
	}

	log.Printf("Batch upserting %d dorms...", len(dormList))
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "name"}},
		DoUpdates: clause.AssignmentColumns([]string{"name"}),
	}).Create(&dormList).Error; err != nil {
		return nil, fmt.Errorf("batch upsert dorms failed: %w", err)
	}

	var allDorms []model.Dorm
	if err := s.db.WithContext(ctx).Find(&allDorms).Error; err != nil {
		return nil, fmt.Errorf("failed to retrieve dorms after upsert: %w", err)
	}

	dormMap := make(map[string]model.Dorm, len(allDorms))
	for _, d := range allDorms {
		dormMap[d.Name] = d
	}
	return dormMap, nil
}

func (s *gormStore) prepareOccupancy(item ApiItem, now time.Time, getStateType func(int) MachineStateType) model.OccupancyOpen {
	var timeRemaining int
	// Use the pre-parsed timestamp from the scraper
	if item.FinishTimeParsed != nil && item.FinishTimeParsed.After(now) {
		timeRemaining = int(item.FinishTimeParsed.Sub(now).Seconds())
	}

	var message string
	switch getStateType(item.State) {
	case StateTypeOccupied:
		message = "使用中"
	case StateTypeFaulty:
		message = "设备故障"
	case StateTypeUnknown:
		message = "未知状态"
	}

	return model.OccupancyOpen{
		MachineID:     item.ID,
		ObservedAt:    now,
		Status:        item.State,
		Message:       message,
		TimeRemaining: timeRemaining,
	}
}

func prepareMachine(item ApiItem, parsedName parse.ParsedName, existingMachines map[int64]model.Machine, dormID int64) (model.Machine, bool) {
	newMachine := model.Machine{
		ID:          item.ID,
		DormID:      dormID,
		DisplayName: item.Name,
		IMEI:        item.IMEI,
		DeviceID:    item.DeviceID,
		FloorCode:   item.FloorCode,
		Floor:       parsedName.Floor,
		Seq:         parsedName.Seq,
	}

	if oldMachine, exists := existingMachines[newMachine.ID]; exists {
		if oldMachine.DisplayName == newMachine.DisplayName &&
			oldMachine.IMEI == newMachine.IMEI &&
			oldMachine.DeviceID == newMachine.DeviceID &&
			oldMachine.FloorCode == newMachine.FloorCode &&
			oldMachine.Floor == newMachine.Floor &&
			oldMachine.Seq == newMachine.Seq {
			return newMachine, false
		}
	}
	return newMachine, true
}

func batchUpsertMachines(tx *gorm.DB, machines []model.Machine) error {
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"dorm_id", "display_name", "imei", "device_id", "floor_code", "floor", "seq", "updated_at"}),
	}).Create(&machines).Error
}
