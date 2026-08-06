function meta()
    return {
        skip        = true,
        name        = "Example Ping",
        description = "Pings 1.1.1.1 to measure latency and packet loss. Needs root.",
    }
end

function check()
    local r = icmp_ping("1.1.1.1", {
        count = 3,
        timeout = 5,
    })

    if r.Error ~= "" then
        r.FailReason = r.Error
        return r
    end

    if r.PacketsReceived == 0 then
        r.FailReason = "No response"
        return r
    end

    if r.PacketLossPct > 0 then
        r.Pass = DEGRADED
        r.FailReason = "Packet loss " .. r.PacketLossPct .. "%"
        return r
    end

    r.Pass = PASS
    return r
end
