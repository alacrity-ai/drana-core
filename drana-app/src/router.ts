import { useState, useEffect } from 'react';

type Route = {
  page: string;
  params: Record<string, string>;
};

function parseHash(): Route {
  const hash = window.location.hash.replace(/^#\/?/, '');
  if (!hash || hash === '/') return { page: 'feed', params: {} };

  const parts = hash.split('/');
  if (parts[0] === 'post' && parts[1]) return { page: 'post', params: { id: parts[1] } };
  if (parts[0] === 'channels') return { page: 'channels', params: {} };
  if (parts[0] === 'rewards') return { page: 'rewards', params: {} };
  if (parts[0] === 'profile' && parts[1]) return { page: 'profile', params: { address: parts[1] } };

  return { page: 'feed', params: {} };
}

export function useRoute(): Route {
  const [route, setRoute] = useState(parseHash);

  useEffect(() => {
    const handler = () => setRoute(parseHash());
    window.addEventListener('hashchange', handler);
    return () => window.removeEventListener('hashchange', handler);
  }, []);

  return route;
}

export function navigate(path: string) {
  window.location.hash = '#/' + path.replace(/^\//, '');
}
