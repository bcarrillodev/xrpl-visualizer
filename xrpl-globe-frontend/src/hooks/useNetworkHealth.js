import { useEffect, useState } from 'react';

export function useNetworkHealth(url = 'http://localhost:8080/network-health') {
  const [networkHealth, setNetworkHealth] = useState(null);
  const [error, setError] = useState(null);

  useEffect(() => {
    let isMounted = true;

    const fetchHealth = async () => {
      try {
        const res = await fetch(url);
        if (!res.ok) {
          throw new Error(`HTTP ${res.status}`);
        }
        const data = await res.json();
        if (!isMounted) return;
        setNetworkHealth(data);
        setError(null);
      } catch (e) {
        if (!isMounted) return;
        setError(e.message);
      }
    };

    fetchHealth();
    const id = setInterval(fetchHealth, 15000);

    return () => {
      isMounted = false;
      clearInterval(id);
    };
  }, [url]);

  return { networkHealth, networkHealthError: error };
}
