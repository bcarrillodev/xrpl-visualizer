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
      if (
        ws.current &&
        (ws.current.readyState === WebSocket.OPEN || ws.current.readyState === WebSocket.CONNECTING)
      ) {
        return;
      }

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
            const locations = [];
            const appendLocation = (point) => {
              if (!Number.isFinite(point?.latitude) || !Number.isFinite(point?.longitude)) {
                return;
              }
              locations.push(point);
            };

            if (Array.isArray(tx.locations)) {
              tx.locations.forEach(appendLocation);
            }

            const amountXRP = formatDropsToXRP(tx.amount);

            if (amountXRP == null) {
              // Skip malformed/non-drop amounts to keep all displayed values in XRP.
              return;
            }

            setTransactions(prev => {
              const newTx = {
                id: tx.hash,
                geoPoints: locations,
                amountDrops: tx.amount,
                amountXRP,
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
      while (!ready && !isUnmounted) {
        await new Promise(resolve => setTimeout(resolve, 1000));
        ready = await checkBackendReady();
      }
      if (isUnmounted) return;
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
