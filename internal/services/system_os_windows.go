package services

import "golang.org/x/sys/windows/registry"

// osName is the Windows edition, version and build from the registry, as
// `winver` reports them: "Windows 11 Pro 24H2 (build 26100.2605)".
func osName() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return windowsName("", "", "", 0)
	}
	defer func() { _ = k.Close() }()
	product, _, _ := k.GetStringValue("ProductName")
	display, _, _ := k.GetStringValue("DisplayVersion")
	if display == "" {
		// Before Windows 10 20H2 the version was ReleaseId ("2004").
		display, _, _ = k.GetStringValue("ReleaseId")
	}
	build, _, _ := k.GetStringValue("CurrentBuildNumber")
	ubr, _, _ := k.GetIntegerValue("UBR")
	return windowsName(product, display, build, ubr)
}
