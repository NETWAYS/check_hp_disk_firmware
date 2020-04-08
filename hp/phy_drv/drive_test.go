package phy_drv

import (
	"github.com/NETWAYS/check_hp_disk_firmware/nagios"
	"github.com/stretchr/testify/assert"
	"testing"
)

const affectedDrive = "VO0480JFDGT"
const affectedDriveFixed = "HPD8"

func TestPhysicalDrive_GetNagiosStatus(t *testing.T) {
	drive := &PhysicalDrive{
		Id:     "1.1",
		Model:  "OTHERDRIVE",
		FwRev:  "HPD1",
		Serial: "ABC123",
		Status: "ok",
		Hours:  1337,
	}

	var status int
	var info string

	// good
	status, info = drive.GetNagiosStatus()
	assert.Equal(t, nagios.OK, status)
	assert.Regexp(t, `\(1\.1 \) model=\w+ serial=ABC123 firmware=HPD1 hours=1337`, info)

	// failed
	drive.Status = "failed"
	status, info = drive.GetNagiosStatus()
	assert.Equal(t, nagios.Critical, status)
	assert.Regexp(t, `\(1\.1 \) model=\w+ serial=ABC123 firmware=HPD1 hours=1337 - status: failed`, info)

	// affected
	drive.Status = "ok"
	drive.Model = affectedDrive
	status, info = drive.GetNagiosStatus()
	assert.Equal(t, nagios.Critical, status)
	assert.Regexp(t, `\(1\.1 \) model=\w+ serial=ABC123 firmware=HPD1 hours=1337 - affected`, info)

	// affected but fixed
	drive.FwRev = affectedDriveFixed
	status, info = drive.GetNagiosStatus()
	assert.Equal(t, nagios.OK, status)
	assert.Regexp(t, `\(1\.1 \) model=\w+ serial=ABC123 firmware=\w+ hours=1337 - .*applied`, info)
}
