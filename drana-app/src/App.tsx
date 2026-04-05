import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { WalletProvider, useWallet } from './wallet/WalletContext';
import { PendingProvider } from './wallet/PendingContext';
import { TopBar } from './components/TopBar';
import { Feed } from './pages/Feed';
import { PostDetail } from './pages/PostDetail';
import { Channels } from './pages/Channels';
import { Profile } from './pages/Profile';
import { Rewards } from './pages/Rewards';
import { useRoute } from './router';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { staleTime: 5000, refetchInterval: 10000 },
  },
});

function AppInner() {
  const { page, params } = useRoute();
  const { address } = useWallet();

  let content;
  switch (page) {
    case 'post':
      content = <PostDetail id={params.id} />;
      break;
    case 'channels':
      content = <Channels />;
      break;
    case 'rewards':
      content = <Rewards />;
      break;
    case 'profile':
      content = <Profile address={params.address} />;
      break;
    default:
      content = <Feed />;
  }

  return (
    <PendingProvider walletAddress={address}>
      <TopBar />
      <main style={{ maxWidth: 'var(--max-width)', margin: '0 auto', padding: '16px 12px' }}>
        {content}
      </main>
    </PendingProvider>
  );
}

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <WalletProvider>
        <AppInner />
      </WalletProvider>
    </QueryClientProvider>
  );
}
