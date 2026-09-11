// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Greenhouse contributors
// SPDX-License-Identifier: Apache-2.0

package redfish

// PowerEquipment represents the PowerEquipment collection
type PowerEquipment struct {
	ODataID   string `json:"@odata.id"`
	ODataType string `json:"@odata.type"`
	Name      string `json:"Name"`
	RackPDUs  struct {
		ODataID string `json:"@odata.id"`
	} `json:"RackPDUs"`
}

// RackPDUsCollection represents the collection of RackPDUs
type RackPDUsCollection struct {
	ODataID      string `json:"@odata.id"`
	ODataType    string `json:"@odata.type"`
	Name         string `json:"Name"`
	Members      []Link `json:"Members"`
	MembersCount int    `json:"Members@odata.count"`
}

// Link represents a Redfish link object
type Link struct {
	ODataID string `json:"@odata.id"`
}

// RackPDU represents a single RackPDU device
type RackPDU struct {
	ODataID         string `json:"@odata.id"`
	ODataType       string `json:"@odata.type"`
	ID              int    `json:"Id"`
	Name            string `json:"Name"`
	EquipmentType   string `json:"EquipmentType"`
	FirmwareVersion string `json:"FirmwareVersion"`
	Manufacturer    string `json:"Manufacturer"`
	Model           string `json:"Model"`
	PartNumber      string `json:"PartNumber"`
	SerialNumber    string `json:"SerialNumber"`
	Version         string `json:"Version"`
	// PowerState      string `json:"PowerState"`
	Status       Status `json:"Status"`
	Metrics      Link   `json:"Metrics"`
	Outlets      Link   `json:"Outlets"`
	OutletGroups Link   `json:"OutletGroups"`
	Mains        Link   `json:"Mains"`
	Branches     Link   `json:"Branches"`
	// PowerSupplies   Link   `json:"PowerSupplies"`
	RatedPowerWatts int    `json:"RatedPowerWatts"`
	UUID            string `json:"UUID"`
	Location        struct {
		Placement struct {
			Row string `json:"Row"`
		} `json:"Placement"`
	} `json:"Location"`
}

// Status represents the status of a Redfish resource
type Status struct {
	State  string `json:"State"`
	Health string `json:"Health"`
}

// PDUMetrics represents the metrics for a PDU
type PDUMetrics struct {
	ODataID            string        `json:"@odata.id"`
	ODataType          string        `json:"@odata.type"`
	ID                 string        `json:"Id"`
	Name               string        `json:"Name"`
	PowerWatts         SensorReading `json:"PowerWatts"`
	EnergykWh          SensorReading `json:"EnergykWh"`
	TemperatureCelsius SensorReading `json:"TemperatureCelsius"`
}

// SensorReading represents a sensor reading with value and metadata
type SensorReading struct {
	DataSourceUri string  `json:"DataSourceUri"`
	Reading       float64 `json:"Reading"`
	ApparentVA    float64 `json:"ApparentVA"`
	ReactiveVAR   float64 `json:"ReactiveVAR"`
	PowerFactor   float64 `json:"PowerFactor"`
}
