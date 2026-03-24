import { walletState } from './state.js';

const SUI_NETWORK = 'sui:testnet';

// Suppress "Origin not allowed" errors from wallet browser extensions
window.addEventListener('unhandledrejection', (e) => {
  if (e.reason && typeof e.reason.message === 'string' && e.reason.message.includes('Origin not allowed')) {
    e.preventDefault();
  }
});

let connectedWallet = null;
let connectedAccount = null;
let suiClient = null;
let discoveredWallets = [];
let unsubscribeWalletEvents = null;
let Transaction = null;
let signAndExecuteTransaction = null;
let paywallPackageId = null;
let sealClient = null;
let SealClientClass = null;
let SessionKeyClass = null;
let EncryptedObjectClass = null;
let fromHex = null;
let toHex = null;
let walrusPublisherUrl = '';
let walrusAggregatorUrl = '';
let walrusEpochs = 5;

function findSlushWallet() {
  return discoveredWallets.find((w) => w.name.toLowerCase().includes('slush'));
}

async function fetchAppConfig() {
  try {
    const res = await fetch('/api/config');
    if (res.ok) {
      const cfg = await res.json();
      paywallPackageId = cfg.paywall_package_id || null;
      return cfg;
    }
  } catch (_) {
    // config fetch is best-effort
  }
  return {};
}

function syncWalletSignal() {
  walletState.value = {
    connected: !!connectedAccount,
    address: connectedAccount ? connectedAccount.address : null,
    balance: walletState.value.balance,
    available: true,
    error: null,
  };
}

async function refreshBalance() {
  if (!connectedAccount || !suiClient) return;
  try {
    const balance = await suiClient.getBalance({ owner: connectedAccount.address });
    const suiAmount = (Number(balance.totalBalance) / 1_000_000_000).toFixed(4);
    walletState.value = { ...walletState.value, balance: suiAmount + ' SUI' };
  } catch (_) {
    walletState.value = { ...walletState.value, balance: '-- SUI' };
  }
}

export async function initWallet() {
  try {
    const suiVer = '1.45.2';
    const sealVer = '0.10.0';
    const [slushMod, walletMod, suiMod, txMod, walletStdMod, sealMod, utilsMod, configResult] = await Promise.all([
      import('https://esm.sh/@mysten/slush-wallet@1.0.3?bundle'),
      import('https://esm.sh/@wallet-standard/app@1.1.0?bundle'),
      import(`https://esm.sh/@mysten/sui@${suiVer}/client`),
      import(`https://esm.sh/@mysten/sui@${suiVer}/transactions`),
      import('https://esm.sh/@mysten/wallet-standard@0.20.1?bundle'),
      import(`https://esm.sh/@mysten/seal@${sealVer}?deps=@mysten/sui@${suiVer}`),
      import(`https://esm.sh/@mysten/sui@${suiVer}/utils`),
      fetchAppConfig(),
    ]);

    const { registerSlushWallet } = slushMod;
    const { getWallets } = walletMod;
    const { SuiClient, getFullnodeUrl } = suiMod;
    Transaction = txMod.Transaction;
    signAndExecuteTransaction = walletStdMod.signAndExecuteTransaction;

    suiClient = new SuiClient({ url: getFullnodeUrl('testnet') });

    SealClientClass = sealMod.SealClient;
    SessionKeyClass = sealMod.SessionKey;
    EncryptedObjectClass = sealMod.EncryptedObject;
    fromHex = utilsMod.fromHex;
    toHex = utilsMod.toHex;
    walrusPublisherUrl = configResult.walrus_publisher_url || '';
    walrusAggregatorUrl = configResult.walrus_aggregator_url || '';

    sealClient = new SealClientClass({
      suiClient,
      serverConfigs: [
        { objectId: '0x73d05d62c18d9374e3ea529e8e0ed6161da1a141a94d3f76ae3fe4e99356db75', weight: 1 },
        { objectId: '0xf5d14a81a982144ae441cd7d64b09027f116a468bd36e7eca494f750591623c8', weight: 1 },
      ],
      verifyKeyServers: false,
    });

    const { get, on } = getWallets();
    discoveredWallets = [...get()];

    const unsubRegister = on('register', (...newWallets) => {
      for (const w of newWallets) {
        if (!discoveredWallets.some((existing) => existing.name === w.name)) {
          discoveredWallets.push(w);
        }
      }
      const savedAddr = sessionStorage.getItem('paylock_wallet_addr');
      if (savedAddr && !connectedWallet && findSlushWallet()) {
        autoReconnect();
      }
    });

    const unsubUnregister = on('unregister', (...removed) => {
      for (const w of removed) {
        discoveredWallets = discoveredWallets.filter((existing) => existing.name !== w.name);
      }
    });

    unsubscribeWalletEvents = () => {
      unsubRegister();
      unsubUnregister();
    };

    try {
      const reg = registerSlushWallet('PayLock Video Streaming', { origin: window.location.origin });
      if (reg && typeof reg.catch === 'function') reg.catch(() => {});
    } catch (_) {
      // Slush wallet registration can fail on unsupported origins
    }

    const savedAddr = sessionStorage.getItem('paylock_wallet_addr');
    if (savedAddr && findSlushWallet()) {
      await autoReconnect();
    }
  } catch (_) {
    walletState.value = {
      connected: false,
      address: null,
      balance: null,
      available: false,
      error: 'Wallet Unavailable',
    };
  }
}

async function autoReconnect() {
  try {
    const slush = findSlushWallet();
    if (!slush) return;
    const connectFeature = slush.features['standard:connect'];
    const result = await connectFeature.connect({ silent: true });
    if (result.accounts && result.accounts.length > 0) {
      connectedWallet = slush;
      connectedAccount = result.accounts[0];
      syncWalletSignal();
      await refreshBalance();
    }
  } catch (_) {
    sessionStorage.removeItem('paylock_wallet_addr');
  }
}

export async function connectWallet() {
  if (connectedWallet) {
    await disconnectWallet();
    return;
  }

  try {
    const slush = findSlushWallet();
    if (!slush) {
      window.open('https://my.slush.app', '_blank');
      return;
    }

    const connectFeature = slush.features['standard:connect'];
    const result = await connectFeature.connect();

    if (result.accounts.length > 0) {
      connectedWallet = slush;
      connectedAccount = result.accounts[0];
      sessionStorage.setItem('paylock_wallet_addr', connectedAccount.address);
      syncWalletSignal();
      await refreshBalance();
    }
  } catch (_) {
    walletState.value = { ...walletState.value, error: 'Connection Failed' };
    setTimeout(() => {
      walletState.value = { ...walletState.value, error: null };
    }, 2000);
  }
}

export async function disconnectWallet() {
  if (connectedWallet) {
    try {
      const disconnectFeature = connectedWallet.features['standard:disconnect'];
      if (disconnectFeature) await disconnectFeature.disconnect();
    } catch (_) {
      // disconnect is best-effort
    }
  }
  connectedWallet = null;
  connectedAccount = null;
  sessionStorage.removeItem('paylock_wallet_addr');
  walletState.value = { connected: false, address: null, balance: null, available: true, error: null };
}

export async function createVideoOnChain(videoId, price, previewBlobId, fullBlobId, sealNamespace) {
  if (!connectedWallet || !connectedAccount) throw new Error('Wallet not connected');
  if (!paywallPackageId) throw new Error('Paywall contract not configured');
  if (!Transaction || !signAndExecuteTransaction) throw new Error('Sui SDK not loaded');
  if (!/^0x[0-9a-fA-F]{64}$/.test(paywallPackageId)) throw new Error('Invalid paywall package ID format');

  const tx = new Transaction();
  tx.moveCall({
    target: paywallPackageId + '::paywall::create_video',
    arguments: [
      tx.pure.u64(price),
      tx.pure.string(previewBlobId),
      tx.pure.string(fullBlobId),
      tx.pure.vector('u8', sealNamespace || []),
    ],
  });

  const result = await signAndExecuteTransaction(connectedWallet, {
    transaction: tx,
    account: connectedAccount,
    chain: SUI_NETWORK,
  });

  const txResponse = await suiClient.waitForTransaction({
    digest: result.digest,
    options: { showObjectChanges: true },
  });

  const videoType = paywallPackageId + '::paywall::Video';
  const created = (txResponse.objectChanges || []).find(
    (c) => c.type === 'created' && c.objectType === videoType,
  );

  if (!created) throw new Error('Video object not found in transaction result');

  const suiObjectId = created.objectId;

  const res = await fetch('/api/videos/' + encodeURIComponent(videoId) + '/sui-object', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ sui_object_id: suiObjectId }),
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error('Failed to save Sui object ID: ' + (body.error || res.status));
  }

  return suiObjectId;
}

export async function encryptAndPublish(fileData, onProgress) {
  if (!connectedWallet || !connectedAccount) throw new Error('Wallet not connected');
  if (!sealClient || !toHex) throw new Error('Seal SDK not loaded');

  const namespace = crypto.getRandomValues(new Uint8Array(32));
  const nonce = crypto.getRandomValues(new Uint8Array(5));
  const id = toHex(new Uint8Array([...namespace, ...nonce]));

  if (onProgress) onProgress('encrypt');
  const { encryptedObject: encryptedBytes } = await sealClient.encrypt({
    threshold: 1,
    packageId: paywallPackageId,
    id,
    data: new Uint8Array(fileData),
  });

  if (onProgress) onProgress('walrus');
  const walrusRes = await fetch(walrusPublisherUrl + '/v1/blobs?epochs=' + walrusEpochs, {
    method: 'PUT',
    body: encryptedBytes,
  });
  if (!walrusRes.ok) throw new Error('Walrus upload failed: ' + walrusRes.status);
  const walrusData = await walrusRes.json();
  const fullBlobId =
    (walrusData.newlyCreated && walrusData.newlyCreated.blobObject && walrusData.newlyCreated.blobObject.blobId) ||
    (walrusData.alreadyCertified && walrusData.alreadyCertified.blobId);
  if (!fullBlobId) throw new Error('Failed to get blob ID from Walrus response');

  return { namespace, fullBlobId };
}

export async function findAccessPass(videoSuiObjectId) {
  if (!connectedAccount || !paywallPackageId) return null;
  const accessPassType = paywallPackageId + '::paywall::AccessPass';
  let cursor = null;
  let hasNext = true;
  while (hasNext) {
    const result = await suiClient.getOwnedObjects({
      owner: connectedAccount.address,
      filter: { StructType: accessPassType },
      options: { showContent: true },
      cursor,
    });
    for (const obj of result.data) {
      if (obj.data && obj.data.content && obj.data.content.fields) {
        if (obj.data.content.fields.video_id === videoSuiObjectId) {
          return obj.data.objectId;
        }
      }
    }
    hasNext = result.hasNextPage;
    cursor = result.nextCursor;
  }
  return null;
}

export async function purchaseVideo(video) {
  if (!connectedWallet || !connectedAccount) throw new Error('Wallet not connected');
  if (!video.sui_object_id) throw new Error('Video not published on-chain');

  const priceInMist = video.price;
  const tx = new Transaction();
  const [paymentCoin] = tx.splitCoins(tx.gas, [priceInMist]);
  tx.moveCall({
    target: paywallPackageId + '::paywall::purchase_and_transfer',
    arguments: [tx.object(video.sui_object_id), paymentCoin],
  });
  const result = await signAndExecuteTransaction(connectedWallet, {
    transaction: tx,
    account: connectedAccount,
    chain: SUI_NETWORK,
  });

  const txResponse = await suiClient.waitForTransaction({
    digest: result.digest,
    options: { showObjectChanges: true },
  });

  const accessPassType = paywallPackageId + '::paywall::AccessPass';
  const created = (txResponse.objectChanges || []).find(
    (c) => c.type === 'created' && c.objectType === accessPassType,
  );

  return created ? created.objectId : null;
}

export async function decryptVideo(video, knownAccessPassId) {
  if (!connectedWallet || !connectedAccount) throw new Error('Wallet not connected');
  if (!sealClient || !SessionKeyClass || !EncryptedObjectClass) throw new Error('Seal SDK not loaded');
  if (!video.full_blob_url) throw new Error('Encrypted blob not available — upload may have failed');

  const sessionKey = await SessionKeyClass.create({
    address: connectedAccount.address,
    packageId: paywallPackageId,
    ttlMin: 10,
    suiClient,
  });

  const message = sessionKey.getPersonalMessage();
  const signFeature = connectedWallet.features['sui:signPersonalMessage'];
  if (!signFeature) throw new Error('Wallet does not support signPersonalMessage');
  const signResult = await signFeature.signPersonalMessage({
    message,
    account: connectedAccount,
  });
  sessionKey.setPersonalMessageSignature(signResult.signature);

  const encryptedRes = await fetch(video.full_blob_url);
  if (!encryptedRes.ok) throw new Error('Failed to fetch encrypted blob (HTTP ' + encryptedRes.status + ')');
  const encryptedData = new Uint8Array(await encryptedRes.arrayBuffer());
  if (encryptedData.length > 0 && encryptedData[0] === 0x7B) {
    const text = new TextDecoder().decode(encryptedData);
    throw new Error('Walrus returned an error instead of encrypted data: ' + text.slice(0, 200));
  }

  const parsedEncrypted = EncryptedObjectClass.parse(encryptedData);
  const sealId = parsedEncrypted.id;

  const accessPassId = knownAccessPassId || (await findAccessPass(video.sui_object_id));
  if (!accessPassId) throw new Error('No AccessPass found for this video');

  const tx = new Transaction();
  tx.moveCall({
    target: paywallPackageId + '::paywall::seal_approve',
    arguments: [
      tx.pure.vector('u8', fromHex(sealId)),
      tx.object(accessPassId),
      tx.object(video.sui_object_id),
    ],
  });
  const txBytes = await tx.build({ client: suiClient, onlyTransactionKind: true });

  const decryptedBytes = await sealClient.decrypt({
    data: encryptedData,
    sessionKey,
    txBytes,
  });

  const blob = new Blob([decryptedBytes], { type: 'video/mp4' });
  return URL.createObjectURL(blob);
}

export function isWalletConnected() {
  return !!connectedAccount;
}

export function getWalletAddress() {
  return connectedAccount ? connectedAccount.address : null;
}

export function getPaywallPackageId() {
  return paywallPackageId;
}
