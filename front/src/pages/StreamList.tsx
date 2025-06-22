import { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';

interface StreamsResponse {
    error_code: number;
    message: string;
    data: {
        streams: string[];
    };
}

function StreamList() {
    const [streams, setStreams] = useState<string[]>([]);
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        const fetchStreams = async () => {
            try {
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
            } catch (e: any) {
                setError(e.message);
                console.error("Error fetching streams:", e);
            }
        };

        fetchStreams();
    }, []);

    if (error) {
        return <div>Error: {error}</div>;
    }

    return (
        <div>
            <h1>Available Streams</h1>
            {streams.length > 0 ? (
                <ul>
                    {streams.map(streamId => (
                        <li key={streamId}>
                            <Link to={`/player/${streamId}`}>{streamId}</Link>
                        </li>
                    ))}
                </ul>
            ) : (
                <p>No active streams found.</p>
            )}
        </div>
    );
}

export default StreamList; 