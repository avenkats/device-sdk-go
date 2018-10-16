// -*- Mode: Go; indent-tabs-mode: t -*-
//
// Copyright (C) 2017-2018 Canonical Ltd
// Copyright (C) 2018 IOTech Ltd
//
// SPDX-License-Identifier: Apache-2.0

package provision

import (
	"fmt"
	"github.com/edgexfoundry/device-sdk-go/internal/cache"

	"github.com/edgexfoundry/device-sdk-go/internal/common"
	"github.com/edgexfoundry/edgex-go/pkg/models"
	"gopkg.in/mgo.v2/bson"
)

func LoadSchedulesAndEvents(config *common.Config) error {
	err := createSchedules(config.Schedules)
	if err != nil {
		return err
	}

	err = createScheduleEvents(config.ScheduleEvents)
	return err
}

func createSchedules(schedules []models.Schedule) error {
	for i := 0; i < len(schedules); i++ {
		schedule := schedules[i]

		if isScheduleExist(schedule.Name) {
			common.LogCli.Info(fmt.Sprintf("Schedule (%v) exist.", schedule.Name))
			continue
		}

		id, err := common.SchCli.Add(&schedule)
		if err != nil {
			common.LogCli.Error(fmt.Sprintf("Add schedule (%v) fail: %v", schedule.Name, err.Error()))
			return err
		}
		if err = common.VerifyIdFormat(id, "Schedule"); err != nil {
			return err
		}
		schedule.Id = bson.ObjectIdHex(id)
		err = cache.Schedules().Add(schedule)
		if err != nil {
			return err
		}
		common.LogCli.Info(fmt.Sprintf(fmt.Sprintf("Add schedule (%v) successful", schedule.Name)))
	}
	return nil
}

func isScheduleExist(scheduleName string) bool {
	_, isExist := cache.Schedules().ForName(scheduleName)
	return isExist
}

func createScheduleEvents(scheduleEvents []models.ScheduleEvent) error {
	for i := 0; i < len(scheduleEvents); i++ {
		scheduleEvent := scheduleEvents[i]
		if scheduleEvent.Service == "" {
			scheduleEvent.Service = common.ServiceName
		}

		if isScheduleEventExist(scheduleEvent.Name) {
			common.LogCli.Info(fmt.Sprintf("Schedule evnt (%v) exist", scheduleEvent.Name))
			continue
		}

		err := createScheduleEventAddressable(&scheduleEvent)
		if err != nil {
			common.LogCli.Error(fmt.Sprintf("Add schedule event addressable (%v) fail: %v", scheduleEvent.Addressable.Name, err.Error()))
			return err
		}

		id, err := common.SchEvtCli.Add(&scheduleEvent)
		if err != nil {
			common.LogCli.Error(fmt.Sprintf("Add schedule event (%v) fail: %v", scheduleEvent.Name, err.Error()))
			return err
		}
		if err = common.VerifyIdFormat(id, "Schedule Event"); err != nil {
			return err
		}
		scheduleEvent.Id = bson.ObjectIdHex(id)
		err = cache.ScheduleEvents().Add(scheduleEvent)
		if err != nil {
			return err
		}
		common.LogCli.Info(fmt.Sprintf(fmt.Sprintf("Add schedule event (%v) successful", scheduleEvent.Name)))
	}
	return nil
}

func createScheduleEventAddressable(scheduleEvent *models.ScheduleEvent) error {
	scheduleEvent.Addressable.Name = fmt.Sprintf("addressable-%v", scheduleEvent.Name)

	if isScheduleEventAddressableExist(scheduleEvent.Addressable.Name) {
		common.LogCli.Info(fmt.Sprintf("Schedule evnt addressable (%v) exist", scheduleEvent.Addressable.Name))
		return nil
	}

	scheduleEvent.Addressable.Protocol = common.CurrentDeviceService.Addressable.Protocol
	scheduleEvent.Addressable.Address = common.CurrentDeviceService.Addressable.Address
	scheduleEvent.Addressable.Port = common.CurrentDeviceService.Addressable.Port

	addressableId, err := common.AddrCli.Add(&scheduleEvent.Addressable)
	if err != nil {
		return err
	}
	if err = common.VerifyIdFormat(addressableId, "Addressable"); err != nil {
		return err
	}
	scheduleEvent.Addressable.Id = bson.ObjectIdHex(addressableId)

	return nil
}

func isScheduleEventAddressableExist(addressableName string) bool {
	isExist := true
	addressable, _ := common.AddrCli.AddressableForName(addressableName)
	if addressable.Name == "" {
		isExist = false
	}
	return isExist
}

func isScheduleEventExist(scheduleEventName string) bool {
	_, isExist := cache.ScheduleEvents().ForName(scheduleEventName)
	return isExist
}
