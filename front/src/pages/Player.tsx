import MuxPlayer from "@mux/mux-video-react";
import { useParams } from 'react-router-dom';

function Player() {
    const { streamId } = useParams<{ streamId: string }>();

    if (!streamId) {
        return <div>Error: Stream ID not provided.</div>;
    }

    const playbackUrl = `${window.location.protocol}//${window.location.host}/hls/${streamId}/master.m3u8`;

    return (
        <div>
            <h1>Playing: {streamId}</h1>
            <MuxPlayer
                style={{ width: "100%" }}
                src={playbackUrl}
                autoPlay
                muted
                controls
                preferPlayback="mse"
            />
        </div>
    );
}

export default Player; 