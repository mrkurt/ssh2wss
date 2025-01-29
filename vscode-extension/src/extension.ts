import * as vscode from 'vscode';

export async function activate(context: vscode.ExtensionContext) {
    // Register the configure command
    let disposable = vscode.commands.registerCommand('flyssh.configure', async () => {
        const config = await vscode.window.showInputBox({
            prompt: 'Enter your FlySsh configuration URL',
            placeHolder: 'flyssh://hostname?token=abc123',
            ignoreFocusOut: true,
        });

        if (!config) {
            return;
        }

        try {
            // Parse the config URL
            const url = new URL(config);
            if (url.protocol !== 'flyssh:') {
                throw new Error('Invalid configuration URL. Must start with flyssh://');
            }

            // Store the config
            await context.globalState.update('flyssh.config', config);

            // TODO: Configure SSH
            vscode.window.showInformationMessage('FlySsh configured successfully!');
        } catch (error) {
            vscode.window.showErrorMessage(`Invalid configuration: ${error}`);
        }
    });

    context.subscriptions.push(disposable);

    // Watch for Remote SSH errors in the output channel
    const outputChannel = vscode.window.createOutputChannel('Remote SSH');
    context.subscriptions.push(outputChannel);

    context.subscriptions.push(
        vscode.window.onDidOpenTerminal((terminal: vscode.Terminal) => {
            if (terminal.name.includes('Remote SSH')) {
                // Watch the Remote SSH output channel for errors
                const disposable = vscode.workspace.onDidChangeTextDocument(e => {
                    if (e.document.uri.scheme === 'output' && e.document.uri.path.includes('Remote SSH')) {
                        const text = e.document.getText();
                        if (text.toLowerCase().includes('error') || text.toLowerCase().includes('failed')) {
                            vscode.window.showErrorMessage('Remote SSH connection failed', 'Fix with Fly')
                                .then((selection: string | undefined) => {
                                    if (selection === 'Fix with Fly') {
                                        // TODO: Show Fly-specific error handling
                                        vscode.commands.executeCommand('flyssh.configure');
                                    }
                                });
                        }
                    }
                });
                context.subscriptions.push(disposable);
            }
        })
    );

    // Prompt for configuration on first install
    const isConfigured = context.globalState.get('flyssh.config');
    if (!isConfigured) {
        vscode.commands.executeCommand('flyssh.configure');
    }
}

export function deactivate() {} 
