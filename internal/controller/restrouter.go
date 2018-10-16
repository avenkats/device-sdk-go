// -*- Mode: Go; indent-tabs-mode: t -*-
//
// Copyright (C) 2017-2018 Canonical Ltd
// Copyright (C) 2018 IOTech Ltd
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"net/http"

	"github.com/edgexfoundry/device-sdk-go/internal/common"
	"github.com/gorilla/mux"
)

func InitRestRoutes() *mux.Router {
	r := mux.NewRouter().PathPrefix(common.APIPrefix).Subrouter()

	common.LogCli.Debug("init status rest controller")
	r.HandleFunc("/ping", statusFunc)

	common.LogCli.Debug("init command rest controller")
	sr := r.PathPrefix("/device").Subrouter()
	sr.HandleFunc("/{id}/{command}", commandFunc).Methods(http.MethodGet, http.MethodPut)
	sr.HandleFunc("/all/{command}", commandAllFunc).Methods(http.MethodGet, http.MethodPut)

	common.LogCli.Debug("init callback rest controller")
	r.HandleFunc("/callback", callbackFunc)

	common.LogCli.Debug("init other rest controller")
	r.HandleFunc("/discovery", discoveryFunc).Methods("POST")
	r.HandleFunc("/debug/transformData/{transformData}", transformFunc).Methods("GET")

	return r
}
