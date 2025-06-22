import MuxPlayer from "@mux/mux-video-react";
import { useParams, Link, useNavigate } from 'react-router-dom';
import { useState, useEffect } from 'react';
import './Player.css';

interface StreamInfo {
    id: string;
    title: string;
    isLive: boolean;
    viewers?: number;
}

function Player() {
    const { streamId } = useParams<{ streamId: string }>();
    const navigate = useNavigate();
    const [isLoading, setIsLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [streamInfo, setStreamInfo] = useState<StreamInfo | null>(null);
    const [isPlayerReady, setIsPlayerReady] = useState(false);

    useEffect(() => {
        if (!streamId) {
            setError('Stream ID not provided');
            setIsLoading(false);
            return;
        }

        // 스트림 정보 설정 (실제로는 API에서 가져올 수 있음)
        setStreamInfo({
            id: streamId,
            title: streamId,
            isLive: true,
            viewers: Math.floor(Math.random() * 1000) + 1
        });

        setIsLoading(false);
    }, [streamId]);

    const handlePlayerLoadStart = () => {
        setIsPlayerReady(false);
    };

    const handlePlayerLoadedData = () => {
        setIsPlayerReady(true);
    };

    const handlePlayerError = () => {
        setError('Failed to load stream. The stream may be offline or unavailable.');
    };

    const handleGoBack = () => {
        navigate('/');
    };

    if (!streamId) {
        return (
            <div className="player-container">
                <div className="error-container">
                    <h2>Error</h2>
                    <p>Stream ID not provided</p>
                    <Link to="/" className="glass-button">
                        ← Back to Streams
                    </Link>
                </div>
            </div>
        );
    }

    const playbackUrl = `${window.location.protocol}//${window.location.host}/hls/${streamId}/master.m3u8`;

    return (
        <div className="player-container">
            <div className="player-header glass-panel">
                <button onClick={handleGoBack} className="glass-button">
                    ← Back to Streams
                </button>
                <div className="stream-meta">
                    <h1 className="stream-title title-gradient">{streamInfo?.title || streamId}</h1>
                    <div className="stream-stats">
                        {streamInfo?.isLive && (
                            <span className="live-badge">● LIVE</span>
                        )}
                        {streamInfo?.viewers && (
                            <span className="viewer-count">
                                👥 {streamInfo.viewers.toLocaleString()} viewers
                            </span>
                        )}
                    </div>
                </div>
            </div>

            <div className="player-wrapper">
                {isLoading ? (
                    <div className="player-loading glass-panel">
                        <div className="loading-spinner"></div>
                        <p>Loading stream...</p>
                    </div>
                ) : error ? (
                    <div className="error-container">
                        <div className="error-icon">⚠️</div>
                        <h3>Stream Unavailable</h3>
                        <p>{error}</p>
                        <div className="error-actions">
                            <button onClick={() => window.location.reload()} className="btn-primary">
                                🔄 Retry
                            </button>
                            <Link to="/" className="btn-secondary">
                                ← Back to Streams
                            </Link>
                        </div>
                    </div>
                ) : (
                    <div className="video-container">
                        {!isPlayerReady && (
                            <div className="video-loading-overlay">
                                <div className="loading-spinner"></div>
                                <p>Loading video...</p>
                            </div>
                        )}
                        <MuxPlayer
                            className="video-player"
                            src={playbackUrl}
                            autoPlay
                            muted
                            controls
                            preferPlayback="mse"
                            onLoadStart={handlePlayerLoadStart}
                            onLoadedData={handlePlayerLoadedData}
                            onError={handlePlayerError}
                        />
                    </div>
                )}
            </div>

            <div className="player-info grid-responsive grid-2">
                <div className="stream-details glass-panel">
                    <h3>Stream Information</h3>
                    <div className="detail-item">
                        <span className="detail-label">Stream ID:</span>
                        <span className="detail-value">{streamId}</span>
                    </div>
                    <div className="detail-item">
                        <span className="detail-label">Status:</span>
                        <span className="detail-value">
                            {streamInfo?.isLive ? (
                                <span className="status-live">🔴 Live</span>
                            ) : (
                                <span className="status-offline">⚫ Offline</span>
                            )}
                        </span>
                    </div>
                    <div className="detail-item">
                        <span className="detail-label">Quality:</span>
                        <span className="detail-value">Auto (HLS)</span>
                    </div>
                </div>
            </div>
        </div>
    );
}

export default Player; 