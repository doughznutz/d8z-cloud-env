const axios = require('axios');
const inquirer = require('inquirer');
const { spawn } = require('child_process');

const MCP_SERVER_URL = 'http://127.0.0.1:8000';
let pythonProcess;

// Graceful exit
process.on('SIGINT', () => {
    if (pythonProcess) pythonProcess.kill('SIGINT');
    process.exit();
});

// --- MCP Tool Caller ---
async function callMcpTool(tool, params = {}) {
    try {
        const response = await axios.post(`${MCP_SERVER_URL}/mcp`, { tool, params });
        return response.data;
    } catch (error) {
        console.error(`
Error calling tool '${tool}':`);
        if (error.response) {
            console.error(`Status: ${error.response.status}`);
            console.error(`Details: ${JSON.stringify(error.response.data.detail, null, 2)}`);
        } else {
            console.error(error.message);
        }
        throw new Error(`MCP tool call failed for '${tool}'`);
    }
}

// --- Server Lifecycle ---
async function startPythonMcpServer() {
    console.log('Starting Python MCP Server...');
    pythonProcess = spawn('/opt/venv_mcp/bin/uvicorn', ['server:app', '--host', '0.0.0.0', '--port', '8000', '--app-dir', '/app/mcp']);
    
    pythonProcess.stdout.on('data', (data) => console.log(`Python Server: ${data.toString().trim()}`));
    pythonProcess.stderr.on('data', (data) => console.error(`Python Server: ${data.toString().trim()}`));

    const maxRetries = 20;
    for (let i = 0; i < maxRetries; i++) {
        try {
            await axios.get(`${MCP_SERVER_URL}/health`);
            console.log('Python MCP Server is healthy.');
            return;
        } catch (error) {
            await new Promise(resolve => setTimeout(resolve, 500));
        }
    }
    throw new Error('Timed out waiting for Python MCP server.');
}

// --- Setup Sequences ---
async function sequenceUserSetup() {
    console.log('\n--- Verifying User Configuration ---');
    const result = await callMcpTool('check_user_configuration');

    if (result.status === 'success') {
        console.log(`✅ ${result.message}`);
        return; // User is configured, we can proceed.
    }

    if (result.error_code === 'ENV_FILE_MISSING') {
        console.log('INFO: .env file not found. A new one will be created.');
        await callMcpTool('create_env_file');
    }

    console.log("⚠️ The 'USER' variable must be set in your .env file.");
    const { username } = await inquirer.prompt({
        type: 'input', name: 'username', message: 'Please enter your host machine username (e.g., `whoami`):',
    });
    await callMcpTool('update_env_var', { key: 'USER', value: username });
    
    console.log('\nIMPORTANT: Your .env file has been updated.');
    console.log('Please exit and restart the container for these changes to take effect.');
    if ((await inquirer.prompt({ type: 'confirm', name: 'c', message: 'Exit now?', default: true })).c) {
        process.exit(0);
    }
}

async function sequencePrerequisiteChecks() {
    console.log('\n--- Running Prerequisite Checks ---');
    const checks = ['check_docker_socket', 'check_projects_root_directory'];
    for (const check of checks) {
        const result = await callMcpTool(check);
        if (result.status === 'success') {
            console.log(`✅ ${result.message}`);
        } else {
            console.error(`❌ Prerequisite check failed: ${result.message}`);
            console.log('Exiting. Please fix the issue and restart the container.');
            process.exit(1);
        }
    }
}

async function sequenceProjectSetup() {
    console.log('\n--- Configuring Project ---');
    let projectList = (await callMcpTool('list_projects')).projects;
    let projectName;

    const createNewProject = async () => {
        const { p } = await inquirer.prompt({
            type: 'input', name: 'p', message: 'Enter a name for your new project:',
        });
        const { c } = await inquirer.prompt({
            type: 'confirm', name: 'c', message: 'Clone from a Git repository?', default: false,
        });
        if (c) {
            const { repoUrl } = await inquirer.prompt({ type: 'input', name: 'repoUrl', message: 'Enter repository URL:' });
            await callMcpTool('git_clone', { repo_url: repoUrl, project_name: p });
        } else {
            await callMcpTool('create_project_directory', { project_name: p });
        }
        return p;
    };

    if (projectList.length === 0) {
        console.log('No projects found.');
        projectName = await createNewProject();
    } else {
        const choices = [...projectList, new inquirer.Separator(), '<new>'];
        const { p } = await inquirer.prompt({
            type: 'list', name: 'p', message: 'Please select your project:', choices: choices,
        });

        if (p === '<new>') {
            projectName = await createNewProject();
        } else {
            projectName = p;
            console.log(`You selected '${projectName}'. Setting it as the current project.`);
        }
    }

    await callMcpTool('update_env_var', { key: 'PROJECT', value: projectName });
    console.log('\nIMPORTANT: The PROJECT variable has been set.');
    console.log('Please restart the container for this change to be fully effective.');
    if ((await inquirer.prompt({ type: 'confirm', name: 'c', message: 'Exit now?', default: true })).c) {
        process.exit(0);
    }
}

// --- Main Execution ---
async function main() {
    try {
        console.log('--- Welcome to the d8z-cloud-env Container! ---');
        await startPythonMcpServer();
        await sequenceUserSetup();
        await sequencePrerequisiteChecks();
        
        // Check if PROJECT is already set by calling the tool.
        const projectResult = await callMcpTool('check_user_configuration'); // This is a bit of a hack, let's assume if USER is set, PROJECT *could* be.
        // A better approach would be a `check_env_var` tool. For now, we'll just check if the `projects` dir is empty or not to decide.
        const projectList = (await callMcpTool('list_projects')).projects;
        const projectEnv = process.env.PROJECT; // This won't be set yet until restart. We need a way to read from .env.
        // Let's refine the logic. The user setup ensures USER is set. The project setup discovers or creates a project.
        // The prompt to exit and restart is key. So, we just run the project setup.
        
        await sequenceProjectSetup();

        console.log('\n✨ Setup is complete but requires a restart. ✨');
        console.log('The container will now exit. Please run the command again.');
        process.exit(0);

    } catch (error) {
        console.error('\nA critical error occurred during the setup process.');
        process.exit(1);
    }
}

main();