"use client";

import { useState, useEffect } from "react";
import { SessionService } from "@/gen/proto/session/v1/session_connect";
import { ClaudeHistoryEntry } from "@/gen/proto/session/v1/session_pb";
import { createPromiseClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";

export default function HistoryBrowserPage() {
  const [entries, setEntries] = useState<ClaudeHistoryEntry[]>([]);
  const [selectedEntry, setSelectedEntry] = useState<ClaudeHistoryEntry | null>(null);
  const [searchQuery, setSearchQuery] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Create gRPC client
  const transport = createConnectTransport({
    baseUrl: window.location.origin,
  });
  const client = createPromiseClient(SessionService, transport);

  // Load history on mount
  useEffect(() => {
    loadHistory();
  }, []);

  const loadHistory = async (query?: string) => {
    try {
      setLoading(true);
      setError(null);
      const response = await client.listClaudeHistory({
        limit: 100,
        searchQuery: query,
      });
      setEntries(response.entries);
    } catch (err) {
      setError(`Failed to load history: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  const loadEntryDetail = async (id: string) => {
    try {
      setError(null);
      const response = await client.getClaudeHistoryDetail({ id });
      if (response.entry) {
        setSelectedEntry(response.entry);
      }
    } catch (err) {
      setError(`Failed to load entry details: ${err}`);
    }
  };

  const handleSearch = () => {
    loadHistory(searchQuery || undefined);
  };

  const formatDate = (timestamp: any) => {
    if (!timestamp) return "N/A";
    const date = new Date(Number(timestamp.seconds) * 1000);
    return date.toLocaleString();
  };

  return (
    <div style={{ padding: "20px", maxWidth: "1400px", margin: "0 auto" }}>
      <h1 style={{ marginBottom: "20px", fontSize: "24px", fontWeight: "bold" }}>
        📚 Claude History Browser
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

      {/* Search bar */}
      <div style={{ marginBottom: "20px", display: "flex", gap: "10px" }}>
        <input
          type="text"
          placeholder="Search history..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && handleSearch()}
          style={{
            flex: 1,
            padding: "10px",
            border: "1px solid #ccc",
            borderRadius: "4px",
            fontSize: "14px",
          }}
        />
        <button
          onClick={handleSearch}
          style={{
            padding: "10px 20px",
            backgroundColor: "#0070f3",
            color: "#fff",
            border: "none",
            borderRadius: "4px",
            cursor: "pointer",
            fontWeight: "600",
          }}
        >
          Search
        </button>
        <button
          onClick={() => {
            setSearchQuery("");
            loadHistory();
          }}
          style={{
            padding: "10px 20px",
            backgroundColor: "#666",
            color: "#fff",
            border: "none",
            borderRadius: "4px",
            cursor: "pointer",
          }}
        >
          Clear
        </button>
      </div>

      <div style={{ display: "flex", gap: "20px" }}>
        {/* Entry list */}
        <div style={{ flex: 1 }}>
          <h2 style={{ marginBottom: "10px", fontSize: "18px", fontWeight: "600" }}>
            History ({entries.length} entries)
          </h2>
          {loading ? (
            <div>Loading...</div>
          ) : (
            <div
              style={{
                display: "flex",
                flexDirection: "column",
                gap: "10px",
                maxHeight: "700px",
                overflowY: "auto",
              }}
            >
              {entries.map((entry) => (
                <div
                  key={entry.id}
                  onClick={() => loadEntryDetail(entry.id)}
                  style={{
                    padding: "15px",
                    border: "1px solid #ccc",
                    borderRadius: "4px",
                    backgroundColor:
                      selectedEntry?.id === entry.id ? "#e0f0ff" : "#fff",
                    cursor: "pointer",
                    transition: "background-color 0.2s",
                  }}
                  onMouseEnter={(e) =>
                    (e.currentTarget.style.backgroundColor =
                      selectedEntry?.id === entry.id ? "#e0f0ff" : "#f9f9f9")
                  }
                  onMouseLeave={(e) =>
                    (e.currentTarget.style.backgroundColor =
                      selectedEntry?.id === entry.id ? "#e0f0ff" : "#fff")
                  }
                >
                  <div style={{ fontWeight: "600", marginBottom: "5px" }}>
                    {entry.name}
                  </div>
                  <div style={{ fontSize: "13px", color: "#666" }}>
                    {formatDate(entry.updatedAt)} • {entry.model} • {entry.messageCount}{" "}
                    messages
                  </div>
                  {entry.project && (
                    <div
                      style={{
                        fontSize: "12px",
                        color: "#888",
                        marginTop: "5px",
                        fontFamily: "monospace",
                      }}
                    >
                      📁 {entry.project}
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Detail panel */}
        <div style={{ width: "400px", flexShrink: 0 }}>
          {selectedEntry ? (
            <div>
              <h2 style={{ marginBottom: "15px", fontSize: "18px", fontWeight: "600" }}>
                Entry Details
              </h2>
              <div style={{ display: "flex", flexDirection: "column", gap: "15px" }}>
                <div>
                  <div style={{ fontWeight: "600", marginBottom: "5px" }}>Name:</div>
                  <div style={{ color: "#333" }}>{selectedEntry.name}</div>
                </div>
                <div>
                  <div style={{ fontWeight: "600", marginBottom: "5px" }}>ID:</div>
                  <div style={{ fontFamily: "monospace", fontSize: "12px", color: "#666" }}>
                    {selectedEntry.id}
                  </div>
                </div>
                {selectedEntry.project && (
                  <div>
                    <div style={{ fontWeight: "600", marginBottom: "5px" }}>Project:</div>
                    <div style={{ fontFamily: "monospace", fontSize: "13px", color: "#0070f3" }}>
                      {selectedEntry.project}
                    </div>
                  </div>
                )}
                <div>
                  <div style={{ fontWeight: "600", marginBottom: "5px" }}>Model:</div>
                  <div style={{ color: "#333" }}>{selectedEntry.model}</div>
                </div>
                <div>
                  <div style={{ fontWeight: "600", marginBottom: "5px" }}>Message Count:</div>
                  <div style={{ color: "#333" }}>{selectedEntry.messageCount}</div>
                </div>
                <div>
                  <div style={{ fontWeight: "600", marginBottom: "5px" }}>Created:</div>
                  <div style={{ fontSize: "13px", color: "#666" }}>
                    {formatDate(selectedEntry.createdAt)}
                  </div>
                </div>
                <div>
                  <div style={{ fontWeight: "600", marginBottom: "5px" }}>Last Updated:</div>
                  <div style={{ fontSize: "13px", color: "#666" }}>
                    {formatDate(selectedEntry.updatedAt)}
                  </div>
                </div>
              </div>
            </div>
          ) : (
            <div
              style={{
                padding: "40px 20px",
                textAlign: "center",
                color: "#888",
                border: "2px dashed #ddd",
                borderRadius: "4px",
              }}
            >
              Select an entry to view details
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
