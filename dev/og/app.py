import json

import requests
import streamlit as st

OLLAMA_URL = "http://localhost:11434"

st.set_page_config(page_title="Ollama Gateway", layout="wide")
st.title("Ollama Gateway")


def list_models():
    try:
        resp = requests.get(f"{OLLAMA_URL}/api/tags", timeout=5)
        resp.raise_for_status()
        return resp.json().get("models", [])
    except requests.RequestException as e:
        st.error(f"Could not reach Ollama: {e}")
        return []


chat_tab, models_tab = st.tabs(["Chat", "Models"])

with models_tab:
    st.subheader("Installed models")
    models = list_models()
    if models:
        st.table(
            [
                {
                    "name": m["name"],
                    "size_gb": round(m.get("size", 0) / 1e9, 2),
                    "modified": m.get("modified_at", ""),
                }
                for m in models
            ]
        )
    else:
        st.info("No models installed yet.")

    st.subheader("Pull a model")
    pull_name = st.text_input("Model name (e.g. llama3.2:3b)", key="pull_name")
    if st.button("Pull", key="pull_button") and pull_name:
        progress = st.empty()
        try:
            with requests.post(
                f"{OLLAMA_URL}/api/pull",
                json={"name": pull_name, "stream": True},
                stream=True,
                timeout=None,
            ) as resp:
                resp.raise_for_status()
                for line in resp.iter_lines():
                    if not line:
                        continue
                    status = json.loads(line)
                    msg = status.get("status", "")
                    if status.get("total"):
                        pct = status.get("completed", 0) / status["total"] * 100
                        msg = f"{msg} ({pct:.1f}%)"
                    progress.text(msg)
            st.success(f"Pulled {pull_name}")
            st.rerun()
        except requests.RequestException as e:
            st.error(f"Pull failed: {e}")

    if models:
        st.subheader("Remove a model")
        remove_name = st.selectbox("Model", [m["name"] for m in models], key="remove_name")
        if st.button("Delete", key="delete_button"):
            try:
                resp = requests.delete(
                    f"{OLLAMA_URL}/api/delete", json={"name": remove_name}, timeout=10
                )
                resp.raise_for_status()
                st.success(f"Deleted {remove_name}")
                st.rerun()
            except requests.RequestException as e:
                st.error(f"Delete failed: {e}")

with chat_tab:
    model_names = [m["name"] for m in models]
    if not model_names:
        st.warning("No models installed. Pull one from the Models tab first.")
    else:
        selected_model = st.selectbox("Model", model_names, key="chat_model")

        if "chat_history" not in st.session_state:
            st.session_state.chat_history = []

        for msg in st.session_state.chat_history:
            with st.chat_message(msg["role"]):
                st.markdown(msg["content"])

        prompt = st.chat_input("Message")
        if prompt:
            st.session_state.chat_history.append({"role": "user", "content": prompt})
            with st.chat_message("user"):
                st.markdown(prompt)

            with st.chat_message("assistant"):
                placeholder = st.empty()
                full_response = ""
                try:
                    with requests.post(
                        f"{OLLAMA_URL}/api/chat",
                        json={
                            "model": selected_model,
                            "messages": st.session_state.chat_history,
                            "stream": True,
                        },
                        stream=True,
                        timeout=None,
                    ) as resp:
                        resp.raise_for_status()
                        for line in resp.iter_lines():
                            if not line:
                                continue
                            chunk = json.loads(line)
                            token = chunk.get("message", {}).get("content", "")
                            full_response += token
                            placeholder.markdown(full_response)
                            if chunk.get("done"):
                                break
                except requests.RequestException as e:
                    full_response = f"Error talking to Ollama: {e}"
                    placeholder.markdown(full_response)

            st.session_state.chat_history.append({"role": "assistant", "content": full_response})
