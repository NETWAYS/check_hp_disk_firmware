package main

import (
	"errors"
	"fmt"
	"github.com/NETWAYS/check_hp_firmware/hp/cntlr"
	"github.com/NETWAYS/check_hp_firmware/hp/ilo"
	"github.com/NETWAYS/check_hp_firmware/hp/phy_drv"
	"github.com/NETWAYS/check_hp_firmware/nagios"
	"github.com/NETWAYS/check_hp_firmware/snmp"
	"github.com/gosnmp/gosnmp"
	log "github.com/sirupsen/logrus"
	flag "github.com/spf13/pflag"
	"os"
	"time"
)

const Readme = `
Icinga / Nagios check plugin to verify HPE controllers an SSD disks or ilo are not affected by certain vulnerabilities.

**HPE Controllers**

  HPE Smart Array SR Gen10 Controller Firmware Version 2.65 (or later) provided in the (HPE document a00097210) is
  required to prevent a potential data inconsistency on select RAID configurations with Smart Array Gen10 Firmware
  Version 1.98 through 2.62, based on the following scenarios. HPE strongly recommends performing this upgrade at the
  customer's earliest opportunity per the "Action Required" in the table located in the Resolution section.
  Neglecting to perform the recommended resolution could result in potential subsequent errors and potential data
  inconsistency.

The check will alert you with a CRITICAL when the firmware is in the affected range with:

* "if you have RAID 1/10/ADM - update immediately!"
* "if you have RAID 5/6/50/60 - update immediately!"

And it will add a short note when "firmware older than affected" or "firmware has been updated". At the moment the
plugin does not verify configured logical drives, but we believe you should update in any case.

**HPE SSD SAS disks**

  HPE SAS Solid State Drives - Critical Firmware Upgrade Required for Certain HPE SAS Solid State Drive Models to
  Prevent Drive Failure at 32,768 or 40,000 Hours of Operation

The check will raise a CRITICAL when the drive needs to be updated with the note "affected by FW bug", and when
the drive is patched with "firmware update applied".

**HPE Integrated Lights-Out**
  Multiple security vulnerabilities have been identified in Integrated Lights-Out 3 (iLO 3),
  Integrated Lights-Out 4 (iLO 4), and Integrated Lights-Out 5 (iLO 5) firmware. The vulnerabilities could be remotely
  exploited to execute code, cause denial of service, and expose sensitive information. HPE has released updated
  firmware to mitigate these vulnerabilities.

  The check will raise a CRITICAL when the Integrated Lights-Out needs to be updated. Below you will find a list with
  the least version of each Integrated Lights-Out version:
   - HPE Integrated Lights-Out 3 (iLO 3) firmware v1.93 or later.
   - HPE Integrated Lights-Out 4 (iLO 4) firmware v2.75 or later
   - HPE Integrated Lights-Out 5 (iLO 5) firmware v2.18 or later.


Please see support documents from HPE:
* https://support.hpe.com/hpesc/public/docDisplay?docLocale=en_US&docId=emr_na-a00092491en_us
* https://support.hpe.com/hpesc/public/docDisplay?docLocale=en_US&docId=a00097382en_us
* https://support.hpe.com/hpesc/public/docDisplay?docLocale=en_US&docId=a00097210en_us
* https://support.hpe.com/hpesc/public/docDisplay?docId=hpesbhf04012en_us

**IMPORTANT:** Read the documentation for HPE! The plugin and its documentation is a best effort to find and detect
affected hardware. There is ABSOLUTELY NO WARRANTY, see the license!

**Note:** This plugin was initially named check_hp_disk_firmware
`

// Check for HP Controller CVEs via SNMP
func main() {
	flagSet := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	flagSet.SortFlags = false

	host := flagSet.StringP("hostname", "H", "localhost", "SNMP host")
	community := flagSet.StringP("community", "c", "public", "SNMP community")
	protocol := flagSet.StringP("protocol", "P", "2c", "SNMP protocol")
	timeout := flagSet.Int64("timeout", 15, "SNMP timeout in seconds")

	file := flagSet.String("snmpwalk-file", "", "Read output from snmpwalk")

	ignoreIlo := flagSet.Bool("ignore-ilo-version", false, "Don't check the ILO version")
	_ = flagSet.BoolP("ilo", "I", false, "Checks the version of iLo")
	_ = flagSet.MarkHidden("ilo")

	ipv4 := flagSet.BoolP("ipv4", "4", false, "Use IPv4")
	ipv6 := flagSet.BoolP("ipv6", "6", false, "Use IPv6")

	version := flagSet.BoolP("version", "V", false, "Show version")

	debug := flagSet.Bool("debug", false, "Enable debug output")

	flagSet.Usage = func() {
		fmt.Printf("Usage: %s [-H <hostname>] [-c <community>]\n", os.Args[0])
		fmt.Println(Readme)
		fmt.Printf("Version: %s\n", buildVersion())
		fmt.Println()
		fmt.Println("Arguments:")
		flagSet.PrintDefaults()
	}

	err := flagSet.Parse(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			nagios.ExitError(err)
		}

		os.Exit(3)
	}

	if *version {
		fmt.Printf("%s version %s\n", "check_hp_firmware", buildVersion())
		os.Exit(nagios.Unknown)
	}

	if *debug {
		log.SetLevel(log.DebugLevel)
	} else {
		defer nagios.CatchPanic()
	}

	var (
		client     gosnmp.Handler
		cntlrTable *cntlr.CpqDaCntlrTable
		driveTable *phy_drv.CpqDaPhyDrvTable
	)

	if *file != "" {
		client, err = snmp.NewFileHandlerFromFile(*file)
		if err != nil {
			nagios.ExitError(err)
		}
	} else {
		client = gosnmp.NewHandler()
		client.SetTarget(*host)
		client.SetCommunity(*community)
		client.SetTimeout(time.Duration(*timeout) * time.Second)
		client.SetRetries(1)

		version, err := snmp.VersionFromString(*protocol)
		if err != nil {
			nagios.ExitError(err)
		}

		client.SetVersion(version)
	}

	// Initialize connection
	if *ipv4 {
		err = client.ConnectIPv4()
	} else if *ipv6 {
		err = client.ConnectIPv6()
	} else {
		err = client.Connect()
	}

	if err != nil {
		nagios.ExitError(err)
	}

	defer func() {
		_ = client.Close()
	}()

	// Load controller data
	cntlrTable, err = cntlr.GetCpqDaCntlrTable(client)
	if err != nil {
		nagios.ExitError(err)
	}

	// Load drive data
	driveTable, err = phy_drv.GetCpqDaPhyDrvTable(client)
	if err != nil {
		nagios.ExitError(err)
	}

	if len(cntlrTable.Snmp.Values) == 0 {
		nagios.Exit(3, "No HP controller data found!")
	}

	controllers, err := cntlr.GetControllersFromTable(cntlrTable)
	if err != nil {
		nagios.ExitError(err)
	}

	if len(driveTable.Snmp.Values) == 0 {
		nagios.Exit(3, "No HP drive data found!")
	}

	drives, err := phy_drv.GetPhysicalDrivesFromTable(driveTable)
	if err != nil {
		nagios.ExitError(err)
	}

	overall := nagios.Overall{}

	// check the ILO Version unless set
	if !*ignoreIlo {
		iloData, err := ilo.GetIloInformation(client)
		if err != nil {
			nagios.ExitError(err)
		}

		overall.Add(iloData.GetNagiosStatus())
	}

	countControllers := 0

	for _, controller := range controllers {
		controllerStatus, desc := controller.GetNagiosStatus()

		overall.Add(controllerStatus, desc)

		countControllers += 1
	}

	countDrives := 0

	for _, drive := range drives {
		driveStatus, desc := drive.GetNagiosStatus()

		overall.Add(driveStatus, desc)

		countDrives += 1
	}

	var summary string

	status := overall.GetStatus()

	switch status {
	case nagios.OK:
		summary = fmt.Sprintf("All %d controllers and %d drives seem fine", countControllers, countDrives)
	case nagios.Warning:
		summary = fmt.Sprintf("Found %d warnings", overall.Warnings)
	case nagios.Critical:
		summary = fmt.Sprintf("Found %d critical problems", overall.Criticals)
	}

	overall.Summary = summary
	nagios.Exit(status, overall.GetOutput())
}
