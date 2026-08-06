function meta()
    return {
        name        = "TCP Pass",
        description = "Checks TCP connectivity to the test HTTP server port",
        sites       = { alternate = "basic" },
    }
end

function check()
    local r = tcp_connect("127.0.0.1", 19976, {
        timeout = 5,
    })

    if r.Error ~= "" then
        r.FailReason = r.Error
        return r
    end

    r.Pass = PASS
    return r
end
