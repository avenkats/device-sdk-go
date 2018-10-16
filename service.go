// -*- Mode: Go; indent-tabs-mode: t -*-
//
// Copyright (C) 2017-2018 Canonical Ltd
// Copyright (C) 2018 IOTech Ltd
//
// SPDX-License-Identifier: Apache-2.0

// This package provides a basic EdgeX Foundry device service implementation
// meant to be embedded in an application, similar in approach to the builtin
// net/http package.
package device

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/edgexfoundry/device-sdk-go/internal/cache"
	"github.com/edgexfoundry/device-sdk-go/internal/clientinit"
	"github.com/edgexfoundry/device-sdk-go/internal/common"
	"github.com/edgexfoundry/device-sdk-go/internal/controller"
	"github.com/edgexfoundry/device-sdk-go/internal/provision"
	"github.com/edgexfoundry/device-sdk-go/model"
	"github.com/edgexfoundry/edgex-go/pkg/clients/types"
	"github.com/edgexfoundry/edgex-go/pkg/models"
	"gopkg.in/mgo.v2/bson"
)

var (
	svc *Service
)

// A Service listens for requests and routes them to the right command
type Service struct {
	svcInfo      *common.ServiceInfo
	discovery    model.ProtocolDiscovery
	initAttempts int
	initialized  bool
	stopped      bool
	cw           *Watchers
	asyncCh      chan *model.AsyncValues
}

func (s *Service) Name() string {
	return common.ServiceName
}

func (s *Service) Version() string {
	return common.ServiceVersion
}

func (s *Service) Discovery() model.ProtocolDiscovery {
	return s.discovery
}

func (s *Service) AsyncReadings() bool {
	return common.CurrentConfig.Service.EnableAsyncReadings
}

// Start the device service.
func (s *Service) Start(svcInfo *common.ServiceInfo) (err error) {
	s.svcInfo = svcInfo

	err = clientinit.InitDependencyClients()
	if err != nil {
		return err
	}

	err = selfRegister()
	if err != nil {
		err = common.LogCli.Error("Couldn't register to metadata service")
		return err
	}

	// initialize devices, objects & profiles
	cache.InitCache()
	err = provision.LoadProfiles(common.CurrentConfig.Device.ProfilesDir)
	if err != nil {
		err = common.LogCli.Error("Failed to create the pre-defined Device Profiles")
		return err
	}

	err = provision.LoadDevices(common.CurrentConfig.DeviceList)
	if err != nil {
		err = common.LogCli.Error("Failed to create the pre-defined Devices")
		return err
	}

	err = provision.LoadSchedulesAndEvents(common.CurrentConfig)
	if err != nil {
		err = common.LogCli.Error("Failed to create the pre-defined Schedules or Schedule Events")
		return err
	}

	s.cw = newWatchers()

	// initialize driver
	if common.CurrentConfig.Service.EnableAsyncReadings {
		s.asyncCh = make(chan *model.AsyncValues, common.CurrentConfig.Service.AsyncBufferSize)
		go processAsyncResults()
	}
	err = common.Driver.Initialize(common.LogCli, s.asyncCh)
	if err != nil {
		common.LogCli.Error(fmt.Sprintf("Driver.Initialize failure: %v; exiting.", err))
		return err
	}

	// Setup REST API
	r := controller.InitRestRoutes()

	http.TimeoutHandler(nil, time.Millisecond*time.Duration(s.svcInfo.Timeout), "Request timed out")

	// TODO: call ListenAndServe in a goroutine

	common.LogCli.Info(fmt.Sprintf("*Service Start() called, name=%s, version=%s", common.ServiceName, common.ServiceVersion))
	common.LogCli.Error(http.ListenAndServe(common.Colon+strconv.Itoa(s.svcInfo.Port), r).Error())
	common.LogCli.Debug("*Service Start() exit")

	return err
}

func selfRegister() error {
	common.LogCli.Debug("Trying to find Device Service: " + common.ServiceName)

	ds, err := common.DevSvcCli.DeviceServiceForName(common.ServiceName)

	if err != nil {
		if _, ok := err.(types.ErrNotFound); ok {
			common.LogCli.Info(fmt.Sprintf("Device Service %s doesn't exist, creating a new one", ds.Name))
			ds, err = createNewDeviceService()
		} else {
			common.LogCli.Error(fmt.Sprintf("DeviceServicForName failed: %v", err))
			return err
		}
	} else {
		common.LogCli.Info(fmt.Sprintf("Device Service %s exists", ds.Name))
	}

	common.LogCli.Debug(fmt.Sprintf("Device Service in Core MetaData: %v", ds))
	common.CurrentDeviceService = ds
	svc.initialized = true
	return nil
}

func createNewDeviceService() (models.DeviceService, error) {
	addr, err := makeNewAddressable()
	if err != nil {
		common.LogCli.Error(fmt.Sprintf("makeNewAddressable failed: %v", err))
		return models.DeviceService{}, err
	}
	millis := time.Now().UnixNano() / int64(time.Millisecond)
	ds := models.DeviceService{
		Service: models.Service{
			Name:           common.ServiceName,
			Labels:         svc.svcInfo.Labels,
			OperatingState: "ENABLED",
			Addressable:    *addr,
		},
		AdminState: "UNLOCKED",
	}
	ds.Service.Origin = millis

	id, err := common.DevSvcCli.Add(&ds)
	if err != nil {
		common.LogCli.Error(fmt.Sprintf("Add Deviceservice: %s; failed: %v", common.ServiceName, err))
		return models.DeviceService{}, err
	}
	if err = common.VerifyIdFormat(id, "Device Service"); err != nil {
		return models.DeviceService{}, err
	}

	// NOTE - this differs from Addressable and Device objects,
	// neither of which require the '.Service'prefix
	ds.Service.Id = bson.ObjectIdHex(id)
	common.LogCli.Debug("New deviceservice Id: " + ds.Service.Id.Hex())

	return ds, nil
}

func makeNewAddressable() (*models.Addressable, error) {
	// check whether there has been an existing addressable
	addr, err := common.AddrCli.AddressableForName(common.ServiceName)
	if err != nil {
		if _, ok := err.(types.ErrNotFound); ok {
			common.LogCli.Info(fmt.Sprintf("Addressable %s doesn't exist, creating a new one", common.ServiceName))
			millis := time.Now().UnixNano() / int64(time.Millisecond)
			addr = models.Addressable{
				BaseObject: models.BaseObject{
					Origin: millis,
				},
				Name:       common.ServiceName,
				HTTPMethod: http.MethodPost,
				Protocol:   common.HttpProto,
				Address:    svc.svcInfo.Host,
				Port:       svc.svcInfo.Port,
				Path:       common.APICallbackRoute,
			}
			id, err := common.AddrCli.Add(&addr)
			if err != nil {
				common.LogCli.Error(fmt.Sprintf("Add addressable failed %v, error: %v", addr, err))
				return nil, err
			}
			if err = common.VerifyIdFormat(id, "Addressable"); err != nil {
				return nil, err
			}
			addr.Id = bson.ObjectIdHex(id)
		} else {
			common.LogCli.Error(fmt.Sprintf("AddressableForName failed: %v", err))
			return nil, err
		}
	} else {
		common.LogCli.Info(fmt.Sprintf("Addressable %s exists", common.ServiceName))
	}

	return &addr, nil
}

// Stop shuts down the Service
func (s *Service) Stop(force bool) error {
	s.stopped = true
	common.Driver.Stop(force)
	return nil
}

// NewService create a new device service instance with the given
// name, version and Driver, which cannot be nil.
// Note - this function is a singleton, if called more than once,
// it will alwayd return an error.
func NewService(proto model.ProtocolDriver) (*Service, error) {

	if svc != nil {
		err := fmt.Errorf("NewService: service already exists!\n")
		return nil, err
	}

	if len(common.ServiceName) == 0 {
		err := fmt.Errorf("NewService: empty name specified\n")
		return nil, err
	}

	if proto == nil {
		err := fmt.Errorf("NewService: no Driver specified\n")
		return nil, err
	}

	svc = &Service{}
	common.Driver = proto

	return svc, nil
}

// RunningService returns the Service instance which is running
func RunningService() *Service {
	return svc
}
