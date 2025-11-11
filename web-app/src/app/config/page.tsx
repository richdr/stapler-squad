"use client";

import { useState, useEffect } from "react";
import { SessionService } from "@/gen/proto/session/v1/session_connect";
import { ClaudeConfigFile } from "@/gen/proto/session/v1/session_pb";
import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";

export default function ConfigEditorPage() {
  const [configs, setConfigs] = useState<ClaudeConfigFile[]>([]);
  const [selectedConfig, setSelectedConfig] = useState<ClaudeConfigFile | null>(null);
  const [content, setContent] = useState("");
  const [originalContent, setOriginalContent] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);

  // Create gRPC client
  const transport = createConnectTransport({
    baseUrl: window.location.origin,
  });
  const client = createPromiseClient(SessionService, transport);

  // Load configs on mount
  useEffect(() => {
    loadConfigs();
  }, []);

  const loadConfigs = async () => {
    try {
      setLoading(true);
      setError(null);
      const response = await client.listClaudeConfigs({});
      setConfigs(response.configs);
    } catch (err) {
      setError(`Failed to load configs: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  const loadConfig = async (filename: string) => {
    try {
      setError(null);
      const response = await client.getClaudeConfig({ filename });
      if (response.config) {
        setSelectedConfig(response.config);
        setContent(response.config.content);
        setOriginalContent(response.config.content);
      }
    } catch (err) {
      setError(`Failed to load config: ${err}`);
    }
  };

  const saveConfig = async () => {
    if (!selectedConfig) return;

    try {
      setSaving(true);
      setError(null);
      setSuccessMessage(null);

      await client.updateClaudeConfig({
        filename: selectedConfig.name,
        content: content,
      });

      setOriginalContent(content);
      setSuccessMessage(`✓ Saved ${selectedConfig.name}`);

      // Auto-clear success message after 3 seconds
      setTimeout(() => setSuccessMessage(null), 3000);
    } catch (err) {
      setError(`Failed to save config: ${err}`);
    } finally {
      setSaving(false);
    }
  };

  const hasUnsavedChanges = content !== originalContent;

  return (
    <div style={{ padding: "20px", maxWidth: "1200px", margin: "0 auto" }}>
      <h1 style={{ marginBottom: "20px", fontSize: "24px", fontWeight: "bold" }}>
        📝 Claude Config Editor
      </h1>

      {error && (
        <div
          style={{
            padding: "10px",
            marginBottom: "20px",
            backgroundColor: "#fee",
            border: "1px solid #f88",
            borderRadius: "4px",
            color: "#c00",
          }}
        >
          {error}
        </div>
      )}

      {successMessage && (
        <div
          style={{
            padding: "10px",
            marginBottom: "20px",
            backgroundColor: "#efe",
            border: "1px solid #8f8",
            borderRadius: "4px",
            color: "#060",
          }}
        >
          {successMessage}
        </div>
      )}

      <div style={{ display: "flex", gap: "20px" }}>
        {/* File list */}
        <div style={{ width: "250px", flexShrink: 0 }}>
          <h2 style={{ marginBottom: "10px", fontSize: "18px", fontWeight: "600" }}>
            Config Files
          </h2>
          {loading ? (
            <div>Loading...</div>
          ) : (
            <div style={{ display: "flex", flexDirection: "column", gap: "5px" }}>
              {configs.map((config) => (
                <button
                  key={config.name}
                  onClick={() => loadConfig(config.name)}
                  style={{
                    padding: "10px",
                    textAlign: "left",
                    border: "1px solid #ccc",
                    borderRadius: "4px",
                    backgroundColor:
                      selectedConfig?.name === config.name ? "#e0f0ff" : "#fff",
                    cursor: "pointer",
                    fontWeight:
                      selectedConfig?.name === config.name ? "600" : "normal",
                  }}
                >
                  {config.name}
                </button>
              ))}
            </div>
          )}
        </div>

        {/* Editor */}
        <div style={{ flex: 1 }}>
          {selectedConfig ? (
            <>
              <div
                style={{
                  display: "flex",
                  justifyContent: "space-between",
                  alignItems: "center",
                  marginBottom: "10px",
                }}
              >
                <h2 style={{ fontSize: "18px", fontWeight: "600" }}>
                  {selectedConfig.name}
                  {hasUnsavedChanges && (
                    <span style={{ color: "#f80", marginLeft: "10px" }}>
                      [modified]
                    </span>
                  )}
                </h2>
                <div style={{ display: "flex", gap: "10px" }}>
                  <button
                    onClick={saveConfig}
                    disabled={!hasUnsavedChanges || saving}
                    style={{
                      padding: "8px 16px",
                      backgroundColor: hasUnsavedChanges ? "#0070f3" : "#ccc",
                      color: "#fff",
                      border: "none",
                      borderRadius: "4px",
                      cursor: hasUnsavedChanges ? "pointer" : "not-allowed",
                      fontWeight: "600",
                    }}
                  >
                    {saving ? "Saving..." : "Save"}
                  </button>
                  <button
                    onClick={() => setContent(originalContent)}
                    disabled={!hasUnsavedChanges}
                    style={{
                      padding: "8px 16px",
                      backgroundColor: hasUnsavedChanges ? "#f44" : "#ccc",
                      color: "#fff",
                      border: "none",
                      borderRadius: "4px",
                      cursor: hasUnsavedChanges ? "pointer" : "not-allowed",
                    }}
                  >
                    Discard
                  </button>
                </div>
              </div>
              <textarea
                value={content}
                onChange={(e) => setContent(e.target.value)}
                style={{
                  width: "100%",
                  height: "600px",
                  fontFamily: "monospace",
                  fontSize: "14px",
                  padding: "10px",
                  border: "1px solid #ccc",
                  borderRadius: "4px",
                  resize: "vertical",
                }}
              />
            </>
          ) : (
            <div
              style={{
                padding: "40px",
                textAlign: "center",
                color: "#888",
                border: "2px dashed #ddd",
                borderRadius: "4px",
              }}
            >
              Select a config file to edit
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
