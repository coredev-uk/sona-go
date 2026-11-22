// axdump prints the NSAccessibility tree VoiceOver reads from a running
// Sona, one element per line with its index path, and flags what VoiceOver
// would say twice or stop on for nothing (site/src/content/docs/contributing/
// accessibility.md, "Reading the tree VoiceOver reads"). The terminal running
// it needs Accessibility in System Settings → Privacy & Security.
//
//	swiftc -O e2e/axdump/axdump.swift -o /tmp/axdump
//	/tmp/axdump <pid>               the tree, flags after the line they concern
//	/tmp/axdump <pid> press <path>  AXPress on the element at <path> (as 0.7.8)
import ApplicationServices
import Foundation

func attr(_ e: AXUIElement, _ name: String) -> AnyObject? {
    var v: AnyObject?
    return AXUIElementCopyAttributeValue(e, name as CFString, &v) == .success ? v : nil
}

func text(_ v: AnyObject?) -> String? {
    guard let v = v else { return nil }
    if let s = v as? String { return s.isEmpty ? nil : s }
    if let n = v as? NSNumber { return n.stringValue }
    return nil
}

func children(_ e: AXUIElement) -> [AXUIElement] {
    (attr(e, "AXChildren") as? [AXUIElement]) ?? []
}

let keys = ["AXRole", "AXSubrole", "AXRoleDescription", "AXTitle", "AXDescription", "AXValue",
            "AXValueDescription", "AXHelp", "AXPlaceholderValue", "AXEnabled"]
let controls: Set<String> = ["AXButton", "AXCheckBox", "AXRadioButton", "AXMenuButton", "AXPopUpButton",
                             "AXLink", "AXSlider", "AXTextField", "AXComboBox"]

// startsWithWords reports whether s starts with the whole words of p: "Play
// OK Computer" with "Play", not "Shuffle off" with the shortcut "S".
func startsWithWords(_ s: String, _ p: String) -> Bool {
    guard s.lowercased().hasPrefix(p.lowercased()) else { return false }
    let rest = s.dropFirst(p.count)
    return rest.first.map { !$0.isLetter && !$0.isNumber } ?? true
}

// flags are what VoiceOver would read twice, read as a number, or stop on
// with nothing to say.
func flags(_ a: [String: String]) -> [String] {
    let role = a["AXRole"] ?? ""
    let name = a["AXTitle"] ?? a["AXDescription"] ?? ""
    var out: [String] = []
    if let help = a["AXHelp"], !name.isEmpty, startsWithWords(name, help) || startsWithWords(help, name) {
        out.append("the hint repeats the name")
    }
    if role == "AXStaticText", name.rangeOfCharacter(from: .alphanumerics) == nil {
        out.append("a text stop with no words")
    }
    if role == "AXSlider", a["AXValueDescription"] == nil, a["AXHelp"] != nil {
        out.append("the slider's value text is a hint, not its value")
    }
    // AppKit's own window buttons (close, minimise, full screen) have a
    // subrole VoiceOver names them by.
    if controls.contains(role), name.isEmpty, a["AXSubrole"] == nil {
        out.append("an unnamed control")
    }
    if name.range(of: #"^[a-z0-9]+(\.[a-z0-9]+)+$"#, options: .regularExpression) != nil {
        out.append("an SF Symbol's name")
    }
    return out
}

func dump(_ e: AXUIElement, _ path: [Int], _ depth: Int) -> Int {
    var a: [String: String] = [:]
    for k in keys { if let s = text(attr(e, k)) { a[k] = s } }
    let line = keys.compactMap { k in a[k].map { "\(k.dropFirst(2))=\($0.replacingOccurrences(of: "\n", with: "\\n"))" } }
    let indent = String(repeating: "  ", count: depth)
    print(indent + "[\(path.map(String.init).joined(separator: "."))] " + line.joined(separator: " | "))
    var n = 0
    for f in flags(a) {
        print(indent + "  ! " + f)
        n += 1
    }
    for (i, c) in children(e).enumerated() { n += dump(c, path + [i], depth + 1) }
    return n
}

let args = CommandLine.arguments
guard args.count >= 2, let pid = pid_t(args[1]) else {
    FileHandle.standardError.write("usage: axdump <pid> [press <path>]\n".data(using: .utf8)!)
    exit(2)
}
guard AXIsProcessTrusted() else {
    FileHandle.standardError.write("axdump: this terminal needs Accessibility (Privacy & Security)\n".data(using: .utf8)!)
    exit(2)
}
let windows = (attr(AXUIElementCreateApplication(pid), "AXWindows") as? [AXUIElement]) ?? []
guard !windows.isEmpty else {
    FileHandle.standardError.write("axdump: process \(pid) has no windows\n".data(using: .utf8)!)
    exit(1)
}
if args.count >= 4, args[2] == "press" {
    var e = windows[0]
    for part in args[3].split(separator: ".").dropFirst() {
        guard let i = Int(part), i < children(e).count else { print("no element at \(args[3])"); exit(1) }
        e = children(e)[i]
    }
    exit(AXUIElementPerformAction(e, "AXPress" as CFString) == .success ? 0 : 1)
}
var flagged = 0
for (i, w) in windows.enumerated() { flagged += dump(w, [i], 0) }
print("\(flagged) flagged")
exit(flagged == 0 ? 0 : 1)
