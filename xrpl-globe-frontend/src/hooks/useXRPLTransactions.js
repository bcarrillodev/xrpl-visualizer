import { useState, useEffect, useRef } from 'react';

const MAX_TRANSACTIONS = 1000; // Keep last 1000 for visual trails

export function useXRPLTransactions(url, healthUrl = 'http://localhost:8080/health') {
  const [transactions, setTransactions] = useState([]);
  const [isConnected, setIsConnected] = useState(false);
  const [connectionError, setConnectionError] = useState(null);
  const ws = useRef(null);
  const reconnectTimer = useRef(null);
  const reconnectAttempts = useRef(0);
  const maxReconnectDelay = 30000; // 30 seconds

  useEffect(() => {
    let isUnmounted = false;

    function checkBackendReady() {
      return fetch(healthUrl)
        .then(res => res.ok)
        .catch(() => false);
    }

    function connect() {
      if (isUnmounted) return;
      if (ws.current && ws.current.readyState === WebSocket.OPEN) return;

      try {
        ws.current = new WebSocket(url);
        setConnectionError(null);

        ws.current.onopen = () => {
          console.log('Connected to XRPL Stream');
          setIsConnected(true);
          reconnectAttempts.current = 0; // Reset on successful connection
        };

        ws.current.onmessage = (event) => {
          try {
            const tx = JSON.parse(event.data);
            const sourceGeo = tx.source_info || tx.sourceInfo;
            const destGeo = tx.dest_info || tx.destInfo;
            const extraGeo = tx.extra_info || tx.extraInfo;
            const hasSourceGeo = Number.isFinite(sourceGeo?.latitude) && Number.isFinite(sourceGeo?.longitude);
            const hasDestGeo = Number.isFinite(destGeo?.latitude) && Number.isFinite(destGeo?.longitude);
            const hasArcGeo = hasSourceGeo && hasDestGeo;
            const extraGeoPoints = Array.isArray(extraGeo)
              ? extraGeo.filter((point) => Number.isFinite(point?.latitude) && Number.isFinite(point?.longitude))
              : [];
            const amountXRP = formatDropsToXRP(tx.amount);

            if (amountXRP == null) {
              // Skip malformed/non-drop amounts to keep all displayed values in XRP.
              return;
            }

            setTransactions(prev => {
              const newTx = {
                id: tx.hash,
                hasArcGeo,
                sourceGeo,
                destGeo,
                startLat: sourceGeo?.latitude,
                startLng: sourceGeo?.longitude,
                endLat: destGeo?.latitude,
                endLng: destGeo?.longitude,
                extraGeoPoints,
                amountDrops: tx.amount,
                amountXRP,
                stroke: getAmountStroke(amountXRP),
                color: getAmountColor(amountXRP),
                type: tx.transaction_type
              };
              // Add new tx to top, keep list size manageable
              return [newTx, ...prev].slice(0, MAX_TRANSACTIONS);
            });
          } catch (e) {
            console.error("Error parsing transaction:", e);
            setConnectionError(`Failed to parse transaction: ${e.message}`);
          }
        };

        ws.current.onerror = (error) => {
          console.error('WebSocket error:', error);
          setConnectionError('WebSocket connection error');
        };

        ws.current.onclose = (event) => {
          if (isUnmounted) return;
          console.log(`Disconnected from XRPL Stream. Code: ${event.code}, Reason: ${event.reason}`);
          setIsConnected(false);

          // Exponential backoff for reconnection
          reconnectAttempts.current++;
          const delay = Math.min(1000 * Math.pow(2, reconnectAttempts.current), maxReconnectDelay);
          console.log(`Reconnecting in ${delay}ms (attempt ${reconnectAttempts.current})`);
          reconnectTimer.current = setTimeout(connect, delay);
        };

      } catch (error) {
        console.error('Failed to create WebSocket connection:', error);
        setConnectionError(`Failed to create WebSocket: ${error.message}`);
        // Still try to reconnect
        reconnectAttempts.current++;
        const delay = Math.min(1000 * Math.pow(2, reconnectAttempts.current), maxReconnectDelay);
        reconnectTimer.current = setTimeout(connect, delay);
      }
    }

    // Wait for backend to be ready before connecting
    const initConnection = async () => {
      let ready = await checkBackendReady();
      while (!ready) {
        await new Promise(resolve => setTimeout(resolve, 1000));
        ready = await checkBackendReady();
      }
      connect();
    };

    initConnection();

    return () => {
      isUnmounted = true;
      if (reconnectTimer.current) {
        clearTimeout(reconnectTimer.current);
      }
      if (ws.current) ws.current.close();
    };
  }, [url, healthUrl]);

  return { transactions, isConnected, connectionError };
}

function formatDropsToXRP(drops) {
  const n = Number(drops);
  if (!Number.isFinite(n)) return null;
  return n / 1_000_000;
}

function getAmountStroke(xrp) {
  if (!Number.isFinite(xrp) || xrp <= 0) return 0.4;
  const normalized = Math.min(1, Math.log10(xrp) / 8);
  return 0.4 + (normalized * 1.8);
}

function getAmountColor(xrp) {
  if (!Number.isFinite(xrp)) return '#7fd3ff';
  if (xrp >= 1_000_000) return '#ff365e';
  if (xrp >= 100_000) return '#ff8a2a';
  if (xrp >= 10_000) return '#ffdf2b';
  return '#7fd3ff';
}
