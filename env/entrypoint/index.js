const inquirer = require('inquirer');
const { spawn } = require('child_process');
const net = require('net');

const ROUTER_HOST = '127.0.0.1';
const ROUTER_PORT = 3456;
const DEBUG_MODE = process.argv.includes('--debug');

// Graceful exit
process.on('SIGINT', () => {
    process.exit();
});

// --- MCP Router Caller ---
function callRouter(request) {
    return new Promise((resolve, reject) => {
        if (DEBUG_MODE) {
            console.log(`
[DEBUG] Calling router with request: ${JSON.stringify(request)}`);
        }
        
        const client = new net.Socket();
        let responseData = '';

        client.connect(ROUTER_PORT, ROUTER_HOST, () => {
            client.write(JSON.stringify(request) + '\n');
        });

        client.on('data', (data) => {
            responseData += data.toString();
            // Assuming the full response is sent in one go, ending with a newline.
            if (responseData.endsWith('\n')) {
                client.end();
            }
        });

        client.on('close', () => {
            try {
                const response = JSON.parse(responseData);
                if (DEBUG_MODE) {
                    console.log(`[DEBUG] Raw response from router: ${JSON.stringify(response, null, 2)}`);
                }
                if (response.error) {
                    throw new Error(response.error);
                }
                resolve(response.ok ? response.result : response);
            } catch (e) {
                console.error(`Error parsing response from router: ${e.message}`);
                reject(new Error('Failed to communicate with the MCP router.'));
            }
        });

        client.on('error', (err) => {
            console.error(`Socket error: ${err.message}`);
            reject(new Error(`Could not connect to MCP router at ${ROUTER_HOST}:${ROUTER_PORT}`));
        });
    });
}

// --- Setup Sequences ---
async function main() {
    try {
        console.log('--- Welcome to the d8z-cloud-env Container! ---');
        // The router must be started externally. This client connects to it.
        // The downstream server connection is no longer needed as tools are directly loaded by the main router.
        
        await sequenceUserSetup();
        await sequencePrerequisiteChecks();
        const projectName = await resolveProjectContext();
        await sequenceModeSelection(projectName);

        await showMainMenu();

    } catch (error) {
        console.error(`
A critical error occurred during the setup process: ${error.message}`);
        process.exit(1);
    }
}

async function sequenceUserSetup() {
    console.log('\n--- Verifying User Configuration ---');
    let envState = {};
    try {
        const envToolResult = await callRouter({ type: 'run_tool', name: 'ENV_get_environment' });
        envState = envToolResult || {};
    } catch (error) {
        console.error(`Error reading environment: ${error.message}`);
        // Exit because we cannot determine the state of the system
        console.error('Cannot continue without environment information. Exiting.');
        process.exit(1);
    }

    if (envState.USER) {
        console.log(`✅ USER is configured as '${envState.USER}'.`);
        return;
    }

    // Check if the file exists by checking if the object is empty
    if (Object.keys(envState).length === 0) {
        console.log('INFO: .env file not found. A new one will be created.');
        await callRouter({ type: 'run_tool', name: 'ENV_create_dot_env_file' });
    }

    console.log("⚠️ The 'USER' variable must be set in your .env file.");
    const { username } = await inquirer.prompt({
        type: 'input', name: 'username', message: 'Please enter your host machine username (e.g., `whoami`):',
    });
    await callRouter({ type: 'run_tool', name: 'ENV_set_environment_variable', args: { key: 'USER', value: username } });
    
    console.log('\nIMPORTANT: Your .env file has been updated. A restart is required for changes to take effect.');
    if ((await inquirer.prompt({ type: 'confirm', name: 'c', message: 'Exit now to restart?', default: true })).c) {
        process.exit(0);
    }
}

async function sequencePrerequisiteChecks() {
    console.log('\n--- Running Prerequisite Checks ---');
    // Note: 'check_projects_root_directory' was implicitly handled by the new server logic, so it was removed.
    for (const check of ['check_docker_socket']) {
        const result = await callRouter({ type: 'run_tool', name: `DOCKER_${check}` });
        if (result.status !== 'success') {
            console.error(`❌ Prerequisite check failed: ${result.message}`);
            console.log('Exiting. Please fix the issue and restart the container.');
            process.exit(1);
        }
        console.log(`✅ ${result.message}`);
    }
}

async function sequenceInteractiveProjectSetup() {
    console.log('Starting interactive project setup...');
    let projectList = [];
    try {
        const projToolResult = await callRouter({ type: 'run_tool', name: 'PROJECT_get_projects' });
        projectList = projToolResult.projects || [];
    } catch (error) {
        console.error('Could not list projects:', error.message);
    }
    
    let projectName;
    const createNewProject = async () => {
        const { p } = await inquirer.prompt({ type: 'input', name: 'p', message: 'Enter a name for your new project:' });
        await callRouter({ type: 'run_tool', name: 'PROJECT_create_project_directory', args: { project_name: p } });
        return p;
    };

    if (projectList.length === 0) {
        projectName = await createNewProject();
    } else {
        projectList.sort();
        const { p } = await inquirer.prompt({
            type: 'list', name: 'p', message: 'Please select your project:', choices: [...projectList, new inquirer.Separator(), '<new>'],
        });
        projectName = (p === '<new>') ? await createNewProject() : p;
    }

    await callRouter({ type: 'run_tool', name: 'ENV_set_environment_variable', args: { key: 'PROJECT', value: projectName } });
    console.log(`✅ Project set to '${projectName}'.`);
    console.log('NOTE: A container restart is recommended for other services to pick up the new PROJECT variable.');
    return projectName;
}

async function resolveProjectContext() {
    console.log('\n--- Resolving Project Context ---');
    let envState = {};
    try {
        const envToolResult = await callRouter({ type: 'run_tool', name: 'ENV_get_environment' });
        envState = envToolResult || {};
    } catch (error) {
        console.error(`Error reading environment: ${error.message}`);
    }

    const projectName = envState.PROJECT;

    if (projectName) {
        const projToolResult = await callRouter({ type: 'run_tool', name: 'PROJECT_get_projects' });
        if (projToolResult.projects && projToolResult.projects.includes(projectName)) {
            console.log(`✅ Project '${projectName}' is set and directory exists.`);
            return projectName;
        }
        
        console.log(`⚠️ Project is set to '${projectName}', but its directory was not found.`);
        const { copySelf } = await inquirer.prompt({
            type: 'confirm',
            name: 'copySelf',
            message: `Do you want to create this project by copying the contents of this container?`,
            default: false,
        });

        if (copySelf) {
            console.log(`Copying from /self to project '${projectName}'...`);
            const result = await callRouter({ type: 'run_tool', name: 'PROJECT_copy_self_to_project', args: { project_name: projectName } });
            if (result.status === 'success') {
                console.log(`✅ ${result.message}`);
                return projectName;
            } else {
                console.error(`❌ Failed to copy project: ${result.message}`);
                console.log('Proceeding to interactive setup...');
            }
        } else {
            console.log('Starting interactive setup...');
        }
    }
    
    return await sequenceInteractiveProjectSetup();
}

async function sequenceModeSelection(projectName) {
    console.log('\n--- Select Your Work Mode ---');
    const { mode } = await inquirer.prompt({
        type: 'list', name: 'mode', message: 'Are you working on development or deployment?',
        choices: ['Development', 'Deployment'],
    });

    if (mode === 'Development') {
        console.log('Proceeding to IDE selection...');
        await sequenceIdeSelection(projectName);
    } else if (mode === 'Deployment') {
        console.log('\n--- Select a Deployment Service ---');
        const serviceChoices = ['cloud', 'docker', 'github'];
        const { service } = await inquirer.prompt({
            type: 'list', name: 'service', message: 'Which service would you like to launch?',
            choices: serviceChoices,
        });

        try {
            console.log(`Launching '${service}' service...`);
            const launchResult = await callRouter({ type: 'run_tool', name: 'DOCKER_launch_service', args: { service_name: service } });

            if (launchResult.status !== 'success') {
                console.error(`❌ Failed to launch service: ${launchResult.message}`);
                if (launchResult.error) console.error(launchResult.error);
                return; // Stop if launch fails
            }
            console.log(`✅ ${launchResult.message}`);

            console.log(`Connecting '${service}' service to the MCP router...`);
            const connectResult = await callRouter({ type: 'run_tool', name: 'ROUTER_connect_service', args: { service_name: service } });

            if (connectResult.status !== 'success') {
                console.error(`❌ Failed to connect service: ${connectResult.message}`);
                if (connectResult.error) console.error(connectResult.error);
                return; // Stop if connect fails
            }
            console.log(`✅ ${connectResult.message}`);
            console.log('The tool registry has been updated. You can now access new tools in the MCP Server Explorer.');

        } catch (error) {
            // Errors from callRouter are already logged, so we just need to stop the sequence.
        } finally {
            console.log('Proceeding to main menu...');
        }
    }
}

async function sequenceIdeSelection(projectName) {
    console.log('\n--- Select Your IDE ---');
    const ideChoices = ['none', 'vnc', 'vscode', 'codeserver'];
    ideChoices.sort();
    const { ide } = await inquirer.prompt({
        type: 'list', name: 'ide', message: 'Which IDE would you like to use?',
        choices: ideChoices,
    });

    if (ide === 'none') {
        console.log('Skipping IDE launch.');
        return;
    }

    console.log(`Launching '${ide}' for project '${projectName}'...`);
    const result = await callRouter({ type: 'run_tool', name: 'DOCKER_launch_ide', args: { ide_name: ide } });

    if (result.status === 'success') {
        console.log(`✅ ${result.message}`);
    } else {
        console.error(`❌ Failed to launch IDE: ${result.message}`);
        console.error(result.error);
    }
}

// --- Interactive Menu ---
async function presentMenu(message, choices, { back = false, exit = false } = {}) {
    const sortedChoices = [...choices].sort();

    let finalChoices = sortedChoices;
    if (back) {
        finalChoices = ['Back', new inquirer.Separator(), ...sortedChoices];
    }
    if (exit) {
        finalChoices.push(new inquirer.Separator(), 'Exit');
    }

    const { selection } = await inquirer.prompt({
        type: 'list',
        name: 'selection',
        message: message,
        choices: finalChoices,
    });
    return selection;
}

async function showMainMenu() {
    while (true) {
        console.log('\n--- What would you like to do next? ---');
        const choice = await presentMenu('Please select an option:', ['Shell', 'Explore MCP Server'], { exit: true });

        if (choice === 'Shell') {
            await new Promise((resolve) => {
                const shell = spawn('/bin/bash', [], { stdio: 'inherit' });
                shell.on('close', resolve);
            });
        } else if (choice === 'Explore MCP Server') {
            await showExplorerMenu();
        } else if (choice === 'Exit') {
            console.log('Exiting...');
            return;
        }
    }
}

async function showExplorerMenu() {
    let capabilities;

    const formatForDisplay = (fullName) => {
        const match = fullName.match(/^([A-Z0-9_]+)_([a-zA-Z0-9_].*)$/);
        if (match) {
            const prefix = match[1].toLowerCase().replace(/_/g, ' ');
            const itemName = match[2];
            return `${itemName} (${prefix})`;
        }
        return fullName;
    };

    try {
        capabilities = await callRouter({ type: 'run_tool', name: 'ROUTER_list_registry' });
    } catch (error) {
        console.error('Error fetching server capabilities:', error.message);
        return;
    }

    while (true) {
        console.log('\n--- MCP Server Explorer ---');
        // Dynamically create categories based on what's available
        const availableCategories = [];
        if (capabilities.agents && capabilities.agents.length > 0) availableCategories.push('Agents');
        if (capabilities.resources && capabilities.resources.length > 0) availableCategories.push('Resources');
        if (capabilities.tools && capabilities.tools.length > 0) availableCategories.push('Tools');

        const category = await presentMenu('Select a category to explore:', availableCategories, { back: true });

        if (category === 'Back') return;

        const items = category === 'Agents' ? capabilities.agents :
                      category === 'Resources' ? capabilities.resources :
                      capabilities.tools;
        const itemType = category.slice(0, -1).toLowerCase();

        const displayNameMap = new Map(items.map(item => [formatForDisplay(item.name), item.name]));
        const displayNames = items.map(item => formatForDisplay(item.name));
        
        const message = `Select a ${itemType} to inspect:`;
        const selectedDisplayName = await presentMenu(message, displayNames, { back: true });

        if (selectedDisplayName === 'Back') continue;

        const originalName = displayNameMap.get(selectedDisplayName);
        const selectedItem = items.find(item => item.name === originalName);

        if (!selectedItem) continue;

        if (itemType === 'agent') {
            console.log(`
--- Description for ${selectedDisplayName} ---
${selectedItem.description}`);
        } else if (itemType === 'resource') {
            try {
                const response = await callRouter({ type: 'access_resource', name: selectedItem.name });
                // Displaying state for any potential future resources
                console.log(`
--- State of ${selectedDisplayName} ---
`, response.state);
            } catch (error) {
                console.error(`Error fetching state for '${selectedDisplayName}':`, error.message);
            }
        } else if (itemType === 'tool') {
            await runTool(selectedItem);
        }
    }
}

async function runTool(tool) {
    console.log(`
--- Running Tool: ${tool.name} ---
${tool.description}`);

    const params = {};
    if (tool.parameters) {
        for (const param of tool.parameters) {
            if (param.required) {
                const { value } = await inquirer.prompt({
                    type: 'input',
                    name: 'value',
                    message: `Enter value for required parameter '${param.name}':`,
                });
                params[param.name] = value;
            }
        }
    }

    try {
        const result = await callRouter({ type: 'run_tool', name: tool.name, args: params });
        console.log('\n--- Tool Result ---');
        console.log(JSON.stringify(result, null, 2));
    } catch (error) {
        // Error is already logged by callRouter
    }
}

main();