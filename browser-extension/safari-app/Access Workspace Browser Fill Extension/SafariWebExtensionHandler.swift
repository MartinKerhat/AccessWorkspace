//
//  SafariWebExtensionHandler.swift
//  Access Workspace Browser Fill Extension
//
//  Created by Martin Kerhat on 31.10.2026.
//

import SafariServices
import os.log

// The extension does not use native messaging: background.js talks to the
// web app's API directly. Any message that does arrive gets an empty reply
// and is not logged, so a future caller cannot spill a connect token or a
// credential into the unified log (which every local process can read).
class SafariWebExtensionHandler: NSObject, NSExtensionRequestHandling {

    func beginRequest(with context: NSExtensionContext) {
        os_log(.default, "Ignoring a native message: this extension does not use native messaging.")

        let response = NSExtensionItem()
        response.userInfo = [SFExtensionMessageKey: [String: Any]()]
        context.completeRequest(returningItems: [response], completionHandler: nil)
    }

}
