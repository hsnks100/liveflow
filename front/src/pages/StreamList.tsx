import { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import './StreamList.css';

interface StreamsResponse {
    error_code: number;
    message: string;
    data: {
        streams: string[];
    };
}

interface StreamCardProps {
    streamId: string;
}

function StreamCard({ streamId }: StreamCardProps) {
    // 썸네일 이미지가 로드되지 않을 경우 기본 이미지를 보여주기 위한 상태
    const [imageError, setImageError] = useState(false);

    const handleImageError = () => {
        setImageError(true);
    };

    return (
        <Link to={`/player/${streamId}`} className="stream-card">
            <div className="stream-thumbnail">
                {!imageError ? (
                    <img 
                        src={`/thumbnail/${streamId}`}
                        alt={`${streamId} thumbnail`}
                        onError={handleImageError}
                        loading="lazy"
                    />
                ) : (
                    <div className="thumbnail-placeholder">
                        <div className="play-icon">▶</div>
                    </div>
                )}
            </div>
            <div className="stream-info">
                <h3 className="stream-title">{streamId}</h3>
                <div className="stream-status">
                    <span className="live-badge">● LIVE</span>
                </div>
            </div>
        </Link>
    );
}

function StreamList() {
    const [streams, setStreams] = useState<string[]>([]);
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(true);

    useEffect(() => {
        const fetchStreams = async () => {
            try {
                setLoading(true);
                const response = await fetch('/streams');
                if (!response.ok) {
                    throw new Error(`HTTP error! status: ${response.status}`);
                }
                const result: StreamsResponse = await response.json();
                if (result.error_code === 0 && result.data && result.data.streams) {
                    setStreams(result.data.streams);
                } else {
                    throw new Error(result.message || 'Failed to fetch streams');
                }
            } catch (error) {
                const errorMessage = error instanceof Error ? error.message : 'Unknown error occurred';
                setError(errorMessage);
                console.error("Error fetching streams:", error);
            } finally {
                setLoading(false);
            }
        };

        fetchStreams();
        
        // 30초마다 스트림 목록을 새로고침
        const interval = setInterval(fetchStreams, 30000);
        
        return () => clearInterval(interval);
    }, []);

    if (loading) {
        return (
            <div className="stream-list-container">
                <div className="loading">
                    <div className="loading-spinner"></div>
                    <p>Loading streams...</p>
                </div>
            </div>
        );
    }

    if (error) {
        return (
            <div className="stream-list-container">
                <div className="error-container">
                    <h2>Error</h2>
                    <p>{error}</p>
                    <button onClick={() => window.location.reload()} className="btn-primary">
                        Retry
                    </button>
                </div>
            </div>
        );
    }

    return (
        <div className="stream-list-container">
            <header className="stream-list-header">
                <h1 className="title-gradient text-large">Live Streams</h1>
                <p className="stream-count text-normal">
                    {streams.length > 0 
                        ? `${streams.length} active stream${streams.length > 1 ? 's' : ''}` 
                        : 'No active streams'
                    }
                </p>
            </header>
            
            {streams.length > 0 ? (
                <div className="streams-grid grid-responsive grid-auto">
                    {streams.map(streamId => (
                        <StreamCard key={streamId} streamId={streamId} />
                    ))}
                </div>
            ) : (
                <div className="empty-state">
                    <div className="empty-icon">📺</div>
                    <h2 className="text-medium">No Live Streams</h2>
                    <p>There are no active streams at the moment.</p>
                    <p>Check back later or start streaming!</p>
                </div>
            )}
        </div>
    );
}

export default StreamList; 