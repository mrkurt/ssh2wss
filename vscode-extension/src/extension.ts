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

    // Watch for Remote SSH errors
    context.subscriptions.push(
        vscode.window.onDidOpenTerminal((terminal: vscode.Terminal) => {
            if (terminal.name.includes('Remote SSH')) {
                const listener = terminal.onDidWriteLine((line: string) => {
                    if (line.toLowerCase().includes('error') || line.toLowerCase().includes('failed')) {
                        vscode.window.showErrorMessage('Remote SSH connection failed', 'Fix with Fly')
                            .then((selection: string | undefined) => {
                                if (selection === 'Fix with Fly') {
                                    // TODO: Show Fly-specific error handling
                                    vscode.commands.executeCommand('flyssh.configure');
                                }
                            });
                    }
                });
                context.subscriptions.push(listener);
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
