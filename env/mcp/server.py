from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field
from typing import Dict, Any
from tools import TOOL_REGISTRY

# --- Pydantic Models ---
class McpRequest(BaseModel):
    tool: str
    params: Dict[str, Any] = Field(default_factory=dict)

# --- FastAPI App ---
app = FastAPI(
    title="MCP Server",
    description="A simple server that exposes a set of tools for a client sequencer.",
    version="1.0.0",
)

# --- Endpoints ---
@app.get("/health")
async def health_check():
    """Endpoint to verify that the server is running."""
    return {"status": "ok"}

@app.post("/mcp")
async def execute_tool(request: McpRequest):
    """The main endpoint for executing tools via the Model Context Protocol."""
    tool_function = TOOL_REGISTRY.get(request.tool)
    
    if not tool_function:
        raise HTTPException(status_code=404, detail=f"Tool '{request.tool}' not found.")
    
    try:
        # Execute the tool with the provided parameters
        result = tool_function(**request.params)
        return result
    except (TypeError, ValueError, FileNotFoundError) as e:
        # Handle errors related to bad parameters or file system issues
        raise HTTPException(status_code=400, detail=str(e))
    except subprocess.CalledProcessError as e:
        # Handle errors from external commands
        raise HTTPException(status_code=500, detail=f"Command failed: {e}")
    except Exception as e:
        # Catch any other unexpected errors
        raise HTTPException(status_code=500, detail=f"An unexpected error occurred: {e}")

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8000)