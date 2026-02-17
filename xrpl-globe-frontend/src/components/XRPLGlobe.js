import React, { useEffect, useMemo, useRef, useState } from "react";
import Globe from "react-globe.gl";
import { useXRPLTransactions } from "../hooks/useXRPLTransactions";
import { useNetworkHealth } from "../hooks/useNetworkHealth";

const MODE = {
  FLOW: "flow",
  HEALTH: "health",
};
const FAST_RETRY_MS = 10 * 1000;
const VALIDATOR_REFRESH_MS = 5 * 60 * 1000;
const VALIDATOR_CACHE_KEY = "xrpl_validators_cache_v1";

const XRPLGlobe = () => {
  const globeEl = useRef();
  const [mode, setMode] = useState(MODE.FLOW);
  const [validators, setValidators] = useState([]);
  const [mappedValidators, setMappedValidators] = useState([]);
  const [fetchError, setFetchError] = useState(null);
  const [minPaymentXRP, setMinPaymentXRP] = useState(10000);

  const { transactions, isConnected, connectionError } = useXRPLTransactions(
    "ws://localhost:8080/transactions",
  );
  const { networkHealth, networkHealthError } = useNetworkHealth();

  useEffect(() => {
    let isMounted = true;

    const readValidatorCache = () => {
      try {
        const raw = localStorage.getItem(VALIDATOR_CACHE_KEY);
        if (!raw) return null;
        const parsed = JSON.parse(raw);
        if (!Array.isArray(parsed.validators)) return null;
        return parsed.validators;
      } catch {
        return null;
      }
    };

    const writeValidatorCache = (cachedValidators) => {
      try {
        localStorage.setItem(
          VALIDATOR_CACHE_KEY,
          JSON.stringify({ validators: cachedValidators, updatedAt: Date.now() }),
        );
      } catch {
        // ignore localStorage write failures
      }
    };

    let timeoutId = null;

    const applyValidators = (allValidators) => {
      const all = Array.isArray(allValidators) ? allValidators : [];
      const mapped = all.filter((v) => v.latitude !== 0 || v.longitude !== 0);
      setValidators(all);
      setMappedValidators(mapped);
      writeValidatorCache(all);
    };

    const scheduleFetch = (delayMs) => {
      timeoutId = setTimeout(fetchValidators, delayMs);
    };

    const fetchValidators = () => {
      fetch("http://localhost:8080/validators")
        .then((res) => {
          if (res.status === 304) {
            return { validators: readValidatorCache() || [] };
          }
          if (!res.ok) {
            throw new Error(`HTTP ${res.status}: ${res.statusText}`);
          }
          return res.json();
        })
        .then((data) => {
          if (!isMounted) return;
          applyValidators(data.validators || []);
          setFetchError(null);
          scheduleFetch((data.validators || []).length > 0 ? VALIDATOR_REFRESH_MS : FAST_RETRY_MS);
        })
        .catch((err) => {
          if (!isMounted) return;
          const cached = readValidatorCache();
          if (cached) {
            applyValidators(cached);
          }
          console.error("Failed to fetch validators:", err);
          setFetchError(`Failed to load validators: ${err.message}`);
          scheduleFetch(FAST_RETRY_MS);
        });
    };

    const cached = readValidatorCache();
    if (cached) {
      applyValidators(cached);
    }
    fetchValidators();

    return () => {
      isMounted = false;
      if (timeoutId) {
        clearTimeout(timeoutId);
      }
    };
  }, []);

  useEffect(() => {
    let isMounted = true;

    const fetchHealth = async () => {
      try {
        const res = await fetch("http://localhost:8080/health");
        if (!res.ok) return;
        const data = await res.json();
        if (!isMounted) return;
        const drops = Number(data.min_payment_drops || 10000000000);
        setMinPaymentXRP(drops / 1_000_000);
      } catch (err) {
        console.error("Failed to fetch backend health:", err);
      }
    };

    fetchHealth();
    const id = setInterval(fetchHealth, 10000);

    return () => {
      isMounted = false;
      clearInterval(id);
    };
  }, []);

  useEffect(() => {
    if (globeEl.current) {
      globeEl.current.controls().autoRotate = true;
      globeEl.current.controls().autoRotateSpeed = mode === MODE.FLOW ? 0.5 : 0.25;
    }
  }, [mode]);

  const flowStats = useMemo(() => {
    if (!transactions.length) {
      return { totalXRP: 0, maxXRP: 0, arcCount: 0 };
    }
    const amounts = transactions
      .map((tx) => Number(tx.amountXRP))
      .filter((v) => Number.isFinite(v));
    return {
      totalXRP: amounts.reduce((acc, cur) => acc + cur, 0),
      maxXRP: amounts.reduce((acc, cur) => Math.max(acc, cur), 0),
      arcCount: transactions.filter((tx) => tx.hasArcGeo).length,
    };
  }, [transactions]);

  const server = networkHealth?.server;

  return (
    <div style={{ position: "relative" }}>
      <div
        style={{
          position: "absolute",
          top: 20,
          left: 20,
          zIndex: 2,
          color: "white",
          fontFamily: "monospace",
          pointerEvents: "none",
          background: "rgba(0,0,0,0.55)",
          padding: "10px 12px",
          borderRadius: 8,
          maxWidth: 360,
        }}
      >
        <h2>XRPL Live Visualizer</h2>
        <p>
          Status: <span style={{ color: isConnected ? "#0f0" : "#f00" }}>{isConnected ? "LIVE" : "CONNECTING..."}</span>
        </p>
        <p>Mode: {mode === MODE.FLOW ? "Live Flow" : "Network Health"}</p>
        <p>Validators: {validators.length}</p>
        <p>Mapped Validators: {mappedValidators.length}</p>

        {mode === MODE.FLOW && (
          <>
            <p>Recent Txs: {transactions.length}</p>
            <p>Mapped Arcs: {flowStats.arcCount}</p>
            <p>Filter: Payments ≥ {minPaymentXRP.toLocaleString()} XRP</p>
            <p>Total Visible Flow: {flowStats.totalXRP.toLocaleString(undefined, { maximumFractionDigits: 2 })} XRP</p>
            <p>Largest Visible Tx: {flowStats.maxXRP.toLocaleString(undefined, { maximumFractionDigits: 2 })} XRP</p>
          </>
        )}

        {mode === MODE.HEALTH && (
          <>
            <p>Server State: {server?.server_state || "unknown"}</p>
            <p>Ledger Index: {server?.ledger_index || 0}</p>
            <p>Peers: {server?.peer_count || 0}</p>
            <p>WebSocket Clients: {networkHealth?.websocket_clients || 0}</p>
            <p>Tx Listener: {networkHealth?.transaction_listener_active ? "active" : "inactive"}</p>
          </>
        )}

        {connectionError && <p style={{ color: "#ff6b6b", fontSize: "12px" }}>Connection Error: {connectionError}</p>}
        {fetchError && <p style={{ color: "#ff6b6b", fontSize: "12px" }}>Fetch Error: {fetchError}</p>}
        {networkHealthError && <p style={{ color: "#ff6b6b", fontSize: "12px" }}>Network Health Error: {networkHealthError}</p>}
      </div>

      <div
        style={{
          position: "absolute",
          top: 20,
          right: 20,
          zIndex: 2,
          pointerEvents: "auto",
          display: "flex",
          gap: 8,
        }}
      >
        <button
          onClick={() => setMode(MODE.FLOW)}
          style={{
            border: "1px solid #66d9ff",
            background: mode === MODE.FLOW ? "#66d9ff" : "rgba(0,0,0,0.4)",
            color: mode === MODE.FLOW ? "#111" : "#66d9ff",
            padding: "8px 12px",
            borderRadius: 6,
            fontFamily: "monospace",
            cursor: "pointer",
          }}
        >
          Live Flow
        </button>
        <button
          onClick={() => setMode(MODE.HEALTH)}
          style={{
            border: "1px solid #f6ff00",
            background: mode === MODE.HEALTH ? "#f6ff00" : "rgba(0,0,0,0.4)",
            color: mode === MODE.HEALTH ? "#111" : "#f6ff00",
            padding: "8px 12px",
            borderRadius: 6,
            fontFamily: "monospace",
            cursor: "pointer",
          }}
        >
          Network Health
        </button>
      </div>

      <Globe
        ref={globeEl}
        globeImageUrl="//unpkg.com/three-globe/example/img/earth-night.jpg"
        backgroundImageUrl="//unpkg.com/three-globe/example/img/night-sky.png"
        pointsData={mappedValidators}
        pointLat={(d) => d.latitude}
        pointLng={(d) => d.longitude}
        onPointHover={(point) => {
          if (globeEl.current) {
            globeEl.current.controls().autoRotate = !point;
          }
        }}
        pointColor={() => "#f6ff00"}
        pointAltitude={(d) => (mode === MODE.HEALTH && d.is_active ? 0.03 : 0.02)}
        pointRadius={(d) => (mode === MODE.HEALTH && d.is_active ? 0.55 : 0.5)}
        pointLabel={(d) => `
          <b>${d.name || d.address}</b><br/>
          ${d.city || "Unknown"}, ${d.country_code || "XX"}
        `}
        arcsData={mode === MODE.FLOW ? transactions.filter((tx) => tx.hasArcGeo) : []}
        arcStartLat={(d) => d.startLat}
        arcStartLng={(d) => d.startLng}
        arcEndLat={(d) => d.endLat}
        arcEndLng={(d) => d.endLng}
        arcColor={(d) => d.color}
        arcDashLength={0.4}
        arcDashGap={2}
        arcDashInitialGap={1}
        arcDashAnimateTime={1800}
        arcStroke={(d) => d.stroke || 0.5}
        arcLabel={(d) => `
          <b>${d.type}</b><br/>
          Amount: ${d.amountXRP != null ? `${d.amountXRP.toLocaleString()} XRP` : "N/A"}<br/>
          ${d.id}
        `}
      />
    </div>
  );
};

export default XRPLGlobe;
