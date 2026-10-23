// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
import Cocoa
import WebKit

final class WindowDelegate: NSObject, NSApplicationDelegate, NSWindowDelegate, WKNavigationDelegate, WKUIDelegate, WKScriptMessageHandler, WKDownloadDelegate {
    var window: NSWindow!
    var web: WKWebView!
    var fileMenu: NSMenu!
    var fileMenuItem: NSMenuItem!
    var revealItem: NSMenuItem!
    let startURL: URL
    init(url: URL) { startURL = url; super.init() }
    func applicationDidFinishLaunching(_ notification: Notification) {
        if let executable = Bundle.main.executableURL {
            let iconURL = executable.deletingLastPathComponent().deletingLastPathComponent().appendingPathComponent("Resources/CamelClip.icns")
            if let icon = NSImage(contentsOf: iconURL) { NSApp.applicationIconImage = icon }
        }
        let menu = NSMenu()
        let appItem = NSMenuItem()
        menu.addItem(appItem)
        let appMenu = NSMenu()
        appMenu.addItem(withTitle: "Quit CamelClip", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
        appItem.submenu = appMenu
        let fileItem = NSMenuItem()
        fileMenuItem = fileItem
        menu.addItem(fileItem)
        fileMenu = NSMenu(title: "File")
        revealItem = NSMenuItem(title: "Show in Folder", action: #selector(showOutputFolder), keyEquivalent: "o")
        revealItem.keyEquivalentModifierMask = [.command, .shift]
        revealItem.target = self
        fileMenu.addItem(revealItem)
        fileItem.submenu = fileMenu
        let editItem = NSMenuItem()
        menu.addItem(editItem)
        let edit = NSMenu(title: "Edit")
        for (title, action, key) in [("Undo", "undo:", "z"), ("Cut", "cut:", "x"), ("Copy", "copy:", "c"), ("Paste", "paste:", "v"), ("Select All", "selectAll:", "a")] {
            edit.addItem(withTitle: title, action: Selector(action), keyEquivalent: key)
        }
        editItem.submenu = edit
        NSApp.mainMenu = menu
        let configuration = WKWebViewConfiguration()
        configuration.websiteDataStore = .nonPersistent()
        configuration.userContentController.add(self, name: "filePicker")
        configuration.userContentController.add(self, name: "appearance")
        web = WKWebView(frame: .zero, configuration: configuration)
        web.underPageBackgroundColor = .windowBackgroundColor
        web.navigationDelegate = self
        web.uiDelegate = self
        window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 1240, height: 820), styleMask: [.titled, .closable, .miniaturizable, .resizable], backing: .buffered, defer: false)
        window.title = "CamelClip"
        window.minSize = NSSize(width: 760, height: 540)
        window.contentView = web
        window.delegate = self
        window.center()
        window.makeKeyAndOrderFront(nil)
        web.load(URLRequest(url: startURL))
        NSApp.activate(ignoringOtherApps: true)
    }
    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { true }
    @objc func showOutputFolder() {
        web.evaluateJavaScript("window.dispatchEvent(new Event('camelclip-open-folder'));", completionHandler: nil)
    }
    func webView(_ webView: WKWebView, decidePolicyFor navigationAction: WKNavigationAction, decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
        guard let url = navigationAction.request.url else { decisionHandler(.cancel); return }
        if navigationAction.shouldPerformDownload && (url.scheme == "blob" || url.host == startURL.host && url.port == startURL.port) { decisionHandler(.download); return }
        if url.scheme == startURL.scheme && url.host == startURL.host && url.port == startURL.port { decisionHandler(.allow) }
        else { decisionHandler(.cancel) }
    }
    func userContentController(_ userContentController: WKUserContentController, didReceive message: WKScriptMessage) {
        if message.name == "appearance", let body = message.body as? [String: String], let theme = body["theme"], ["auto", "light", "dark"].contains(theme) {
            NSApp.appearance = theme == "dark" ? NSAppearance(named: .darkAqua) : theme == "light" ? NSAppearance(named: .aqua) : nil
            if let title = body["revealTitle"] { revealItem.title = title }
            if let title = body["fileTitle"] { fileMenu.title = title; fileMenuItem.title = title }
            return
        }
        guard message.name == "filePicker", let body = message.body as? [String: String], let target = body["target"], ["out", "path"].contains(target) else { return }
        let panel = NSOpenPanel()
        panel.canChooseDirectories = body["kind"] == "directory"
        panel.canChooseFiles = !panel.canChooseDirectories
        panel.allowsMultipleSelection = false
        panel.beginSheetModal(for: window) { response in
            if response == .OK, let url = panel.url {
                let detail = ["path": url.path, "target": target]
                if let data = try? JSONSerialization.data(withJSONObject: detail), let json = String(data: data, encoding: .utf8) {
                    self.web.evaluateJavaScript("window.dispatchEvent(new CustomEvent('camelclip-path', {detail: "+json+"}));", completionHandler: nil)
                }
            }
        }
    }
    func webView(_ webView: WKWebView, navigationAction: WKNavigationAction, didBecome download: WKDownload) { download.delegate = self }
    func download(_ download: WKDownload, decideDestinationUsing response: URLResponse, suggestedFilename: String, completionHandler: @escaping (URL?) -> Void) {
        let panel = NSSavePanel()
        panel.nameFieldStringValue = suggestedFilename
        panel.beginSheetModal(for: window) { response in completionHandler(response == .OK ? panel.url : nil) }
    }
    func webView(_ webView: WKWebView, runJavaScriptConfirmPanelWithMessage message: String, initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping (Bool) -> Void) {
        let alert = NSAlert()
        alert.messageText = "CamelClip"
        alert.informativeText = message
        alert.addButton(withTitle: "OK")
        alert.addButton(withTitle: "Cancel")
        alert.beginSheetModal(for: window) { response in completionHandler(response == .alertFirstButtonReturn) }
    }
    func webView(_ webView: WKWebView, runOpenPanelWith parameters: WKOpenPanelParameters, initiatedByFrame frame: WKFrameInfo, completionHandler: @escaping ([URL]?) -> Void) {
        let panel = NSOpenPanel()
        panel.allowsMultipleSelection = parameters.allowsMultipleSelection
        panel.canChooseDirectories = parameters.allowsDirectories
        panel.beginSheetModal(for: window) { response in completionHandler(response == .OK ? panel.urls : nil) }
    }
}
guard CommandLine.arguments.count == 2, let url = URL(string: CommandLine.arguments[1]), url.host == "127.0.0.1" else { exit(2) }
let app = NSApplication.shared
app.setActivationPolicy(.regular)
let delegate = WindowDelegate(url: url)
app.delegate = delegate
app.run()
