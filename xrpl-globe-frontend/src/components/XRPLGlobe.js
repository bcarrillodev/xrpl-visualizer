import React, { useEffect, useState, useRef } from "react";
import Globe from "react-globe.gl";
import { useXRPLTransactions } from "../hooks/useXRPLTransactions";

const XRPLGlobe = () => {
  const globeEl = useRef();
  const [validators, setValidators] = useState([]);
  const [fetchError, setFetchError] = useState(null);
  const { transactions, isConnected, connectionError } = useXRPLTransactions(
    "ws://localhost:8080/transactions",
  );

  // Fetch Validators (Static Data)
  useEffect(() => {
    let isMounted = true;

    const fetchValidators = () => {
      fetch("http://localhost:8080/validators")
        .then((res) => {
          if (!res.ok) {
            throw new Error(`HTTP ${res.status}: ${res.statusText}`);
          }
          return res.json();
        })
        .then((data) => {
          if (!isMounted) return;

          // Filter out validators with missing geo data (0,0)
          const validNodes = data.validators.filter(
            (v) => v.latitude !== 0 || v.longitude !== 0,
          );
          setValidators(validNodes);
          setFetchError(null);
        })
        .catch((err) => {
          if (!isMounted) return;
          console.error("Failed to fetch validators:", err);
          setFetchError(`Failed to load validators: ${err.message}`);
        });
    };

    fetchValidators();
    const intervalId = setInterval(fetchValidators, 10000);

    return () => {
      isMounted = false;
      clearInterval(intervalId);
    };
  }, []);

  // Auto-rotate globe
  useEffect(() => {
    if (globeEl.current) {
      globeEl.current.controls().autoRotate = true;
      globeEl.current.controls().autoRotateSpeed = 0.5;
    }
  }, []);

  return (
    <div style={{ position: "relative" }}>
      <div
        style={{
          position: "absolute",
          top: 20,
          left: 20,
          zIndex: 1,
          color: "white",
          fontFamily: "monospace",
          pointerEvents: "none",
        }}
      >
        <h2>XRPL Live Visualizer</h2>
        <p>
          Status:{" "}
          <span style={{ color: isConnected ? "#0f0" : "#f00" }}>
            {isConnected ? "LIVE" : "CONNECTING..."}
          </span>
        </p>
        <p>Validators: {validators.length}</p>
        <p>Recent Txs: {transactions.length}</p>
        {connectionError && (
          <p style={{ color: "#ff6b6b", fontSize: "12px" }}>
            Connection Error: {connectionError}
          </p>
        )}
        {fetchError && (
          <p style={{ color: "#ff6b6b", fontSize: "12px" }}>
            Fetch Error: {fetchError}
          </p>
        )}
      </div>

      <Globe
        ref={globeEl}
        globeImageUrl="//unpkg.com/three-globe/example/img/earth-night.jpg"
        backgroundImageUrl="//unpkg.com/three-globe/example/img/night-sky.png"
        // Validators (Points)
        pointsData={validators}
        pointLat={(d) => d.latitude}
        pointLng={(d) => d.longitude}
        onPointHover={(point) => {
          if (globeEl.current) {
            globeEl.current.controls().autoRotate = !point;
          }
        }}
        pointColor={() => "#f6ff00"}
        pointAltitude={0.02}
        pointRadius={0.5}
        pointLabel={(d) => `
          <b>${d.city || "Unknown"}, ${d.country_code || "XX"}
        `}
        // Transactions (Arcs)
        arcsData={transactions}
        arcStartLat={(d) => d.startLat}
        arcStartLng={(d) => d.startLng}
        arcEndLat={(d) => d.endLat}
        arcEndLng={(d) => d.endLng}
        arcColor={(d) => d.color}
        arcDashLength={0.4}
        arcDashGap={2}
        arcDashInitialGap={1}
        arcDashAnimateTime={2000}
        arcStroke={0.5}
        arcLabel={(d) => `
          <b>${d.type}</b><br/>
          Amount: ${d.amount || "N/A"}<br/>
          ${d.id}
        `}
      />
    </div>
  );
};

export default XRPLGlobe;
