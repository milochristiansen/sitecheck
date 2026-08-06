function meta()
    return {
        skip        = true,
        name        = "Example UDP Ping",
        description = "Attempts to use a special unprivileged path to ping 1.1.1.1 to measure latency and packet loss. Only works on Linux, etc.",
    }
end

function check()
    local r = icmp_ping("1.1.1.1", {
        count = 3,
        timeout = 5,
        privileged = false,
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
