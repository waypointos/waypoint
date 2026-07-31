#include <atomic>
#include <chrono>
#include <csignal>
#include <cstdint>
#include <fstream>
#include <iostream>
#include <memory>
#include <optional>
#include <thread>
#include <vector>

#include "config/Args.hpp"
#include "control/DiffDriveKinematics.hpp"
#include "control/DriveController.hpp"
#include "control/DriveSafety.hpp"
#include "joint/Sts3215Driver.hpp"
#include "platform/Descriptor.hpp"
#include "messages/alerts.pb.h"
#include "messages/drive.pb.h"
#include "messages/events.pb.h"
#include "mode/ModeMachine.hpp"
#include "nats/Connection.hpp"
#include "nats/UnixTransport.hpp"
#include "rpc/EstopHandler.hpp"
#include "rpc/GetInfoHandler.hpp"
#include "rpc/RecoverHandler.hpp"
#include "rpc/SetModeHandler.hpp"
#include "servo/AuxRotation.hpp"
#include "servo/Discovery.hpp"
#include "servo/LinuxUart.hpp"
#include "servo/ServoControlServer.hpp"
#include "servo/Sts3215Bus.hpp"
#include "servo/Sts3215Frame.hpp"
#include "sim/ServoModel.hpp"
#include "sim/SimControlServer.hpp"
#include "sim/SimUart.hpp"
#include "telemetry/DrivePublisher.hpp"
#include "telemetry/MotorPublisher.hpp"
#include "telemetry/PlatformPublisher.hpp"
#include "telemetry/PowerPublisher.hpp"
#include "telemetry/ServoStateCache.hpp"
#include "util/Clock.hpp"
#include "util/Stamp.hpp"
#include "watchdog/Watchdog.hpp"

using namespace std::chrono_literals;

namespace {
constexpr char kImageVersion[] = "v0.4.0";

// Servo telemetry: core reads every servo on the bus, not just the drive
// wheels. Drive servos are read at the full 20 Hz; other servos (arm/drill)
// are read a bounded few per tick so they can't starve the 50 Hz control loop.
constexpr int kScanPingTimeoutMs = 20;
constexpr int kAuxReadsPerTick = 2;
constexpr int kAuxMaxFailures = 3;
constexpr auto kPowerFreshWindow = std::chrono::seconds(1);
// cmd.drive arrives at 50 Hz during teleop; 300ms (~15 missed ticks) of
// silence means the operator stream is gone. Independent of the 500ms agent
// heartbeat: the agent can be healthy while the browser stream has died.
constexpr auto kDriveCmdTimeout = std::chrono::milliseconds(300);

std::atomic<bool> g_stop{false};

waypoint::v1::Mode toProtoMode(wp::mode::Mode m) {
    switch (m) {
    case wp::mode::Mode::Manual:     return waypoint::v1::MODE_MANUAL;
    case wp::mode::Mode::Safe:       return waypoint::v1::MODE_SAFE;
    case wp::mode::Mode::Estop:      return waypoint::v1::MODE_ESTOP;
    case wp::mode::Mode::Autonomous: return waypoint::v1::MODE_AUTONOMOUS;
    }
    return waypoint::v1::MODE_UNSPECIFIED;
}
}  // namespace

int main(int argc, char** argv) {
    GOOGLE_PROTOBUF_VERIFY_VERSION;
    auto args = wp::config::Args::parse(argc, argv);

    auto descriptorPath = [&]() -> std::string {
        if (!args.descriptorPath.empty()) return args.descriptorPath;
        if (std::ifstream("/data/waypoint/platform.toml").good())
            return "/data/waypoint/platform.toml";
        return "/usr/share/waypoint/platform.toml";
    }();
    std::string descErr;
    auto desc = wp::platform::Descriptor::load(descriptorPath, &descErr);
    if (!desc) {
        std::cerr << "waypoint-core: " << descErr << "\n";
        return 1;
    }
    std::cerr << "waypoint-core: platform descriptor " << descriptorPath
              << " (" << desc->platformId << ", " << desc->joints.size() << " joints)\n";

    std::signal(SIGINT,  [](int) { g_stop.store(true); });
    std::signal(SIGTERM, [](int) { g_stop.store(true); });
    // The agent owns our NATS Unix socket; when it dies, our next write
    // would trigger SIGPIPE -> silent termination, well before the 500ms
    // watchdog can fire. Ignore it so writes return -1/EPIPE instead and
    // the watchdog gets a chance to safe-stop the rover.
    std::signal(SIGPIPE, SIG_IGN);

    auto transport = std::make_unique<wp::nats::UnixTransport>(args.socketPath);
    wp::nats::Connection nc(std::move(transport));
    if (int e = nc.start()) {
        std::cerr << "waypoint-core: nats connect " << args.socketPath << ": errno " << e << "\n";
        return 1;
    }
    std::cerr << "waypoint-core: connected to " << args.socketPath
              << " rover=" << args.roverID << "\n";

    // The descriptor's single driver table (schema v1) selects real vs sim.
    // --servo-mock forces sim; --servo-port overrides the configured port.
    const auto& drvCfg = desc->drivers.begin()->second;
    const bool useSim = args.servoMock || drvCfg.kind == "sim";
    if (!useSim && !args.simTime.empty()) {
        std::cerr << "waypoint-core: --sim-time requires the sim driver\n";
        return 2;
    }
    std::shared_ptr<wp::sim::ServoModel> simModel;
    wp::sim::SimUart* simUartRaw = nullptr;
    std::unique_ptr<wp::servo::Uart> uart;
    if (useSim) {
        std::vector<wp::sim::ServoConfig> cfgs;
        for (const auto& j : desc->joints) {
            cfgs.push_back({.id = j.busId, .wheelMode = j.type == "wheel"});
        }
        simModel = std::make_shared<wp::sim::ServoModel>(std::move(cfgs), 1);
        auto su = std::make_unique<wp::sim::SimUart>(simModel);
        simUartRaw = su.get();
        uart = std::move(su);
        std::cerr << "waypoint-core: sim driver active ("
                  << (args.simTime == "controlled" ? "controlled" : "realtime")
                  << " clock)\n";
    } else {
        uart = std::make_unique<wp::servo::LinuxUart>();
    }
    // Args.servoPort defaults to "/dev/ttyAMA0" (or "mock" under --servo-mock);
    // an explicit --servo-port wins, otherwise the descriptor port applies.
    const std::string servoPort = useSim ? "sim"
        : (!args.servoPort.empty() && args.servoPort != "/dev/ttyAMA0"
               ? args.servoPort
               : (drvCfg.port.empty() ? args.servoPort : drvCfg.port));
    auto bus = std::make_shared<wp::servo::Sts3215Bus>(std::move(uart));
    if (int e = bus->open(servoPort, drvCfg.baud)) {
        std::cerr << "waypoint-core: servo open " << servoPort << ": errno " << e << "\n";
        return 1;
    }

    std::unique_ptr<wp::util::Clock> clock;
    wp::util::SimClock* simClock = nullptr;
    if (useSim) {
        auto sc = std::make_unique<wp::util::SimClock>(
            args.simTime == "controlled" ? wp::util::SimClock::Mode::Controlled
                                         : wp::util::SimClock::Mode::Realtime);
        simClock = sc.get();
        clock = std::move(sc);
        wp::util::setMonoNsSource([simClock] { return simClock->virtualMonoNs(); });
    } else {
        clock = std::make_unique<wp::util::RealClock>();
    }
    auto clockNow = [&clock] { return clock->now(); };

    wp::mode::ModeMachine mode;
    wp::watchdog::Watchdog wd(500ms, clockNow);
    wp::control::DriveSafety driveSafety(kDriveCmdTimeout, clockNow);

    wp::joint::Sts3215Driver jointDrv(bus.get(), *desc, desc->drivers.begin()->first);
    // Drive is a capability: built only when the descriptor declares
    // diff-drive kinematics. Everything drive-related below checks `drive`.
    std::optional<wp::control::WheelJoints> wheels;
    std::optional<wp::control::Kinematics> kin;
    std::optional<wp::control::DriveController> drive;
    if (desc->hasDrive()) {
        auto wheelId = [&](const char* pos) -> wp::joint::JointId {
            auto it = desc->kinematics->wheels.find(pos);
            auto id = jointDrv.idForName(it->second);
            return *id;  // descriptor validation guarantees presence
        };
        wheels.emplace(wp::control::WheelJoints{
            .frontLeft = wheelId("front_left"), .frontRight = wheelId("front_right"),
            .backLeft = wheelId("back_left"),   .backRight = wheelId("back_right")});
        kin.emplace(wp::control::Kinematics{
            .wheelRadius_m = desc->kinematics->wheelRadiusM,
            .trackWidth_m = desc->kinematics->trackWidthM});
        drive.emplace(&jointDrv, *kin, *wheels);
    } else {
        std::cerr << "waypoint-core: platform has no drive (vehicle_class "
                  << desc->vehicleClass << "); drive subsystem disabled\n";
    }

    const std::string modeSubject = "waypoint." + args.roverID + ".event.mode";
    auto publishMode = [&](wp::mode::Mode from, wp::mode::Mode to) {
        waypoint::v1::ModeEvent ev;
        ev.set_from(toProtoMode(from));
        ev.set_to(toProtoMode(to));
        std::string out;
        ev.SerializeToString(&out);
        nc.publish(modeSubject, out.data(), out.size());
    };
    // STS3215 in wheel mode acts on GOAL_SPEED even with TORQUE_ENABLE=0, so
    // we disarm the controller (no writes) on top of disabling torque.
    auto armDrive = [&](bool on) {
        if (!drive) return;
        drive->setArmed(on);
        // Disarming: command zero speed once. A wheel-mode STS3215 acts on a
        // latched GOAL_SPEED even with TORQUE_ENABLE=0, so torque-off alone
        // would let it keep spinning; an explicit zero write stops it.
        if (!on) {
            jointDrv.writeCommands({
                {wheels->frontLeft,  {.velocityRadps = 0.0}},
                {wheels->frontRight, {.velocityRadps = 0.0}},
                {wheels->backLeft,   {.velocityRadps = 0.0}},
                {wheels->backRight,  {.velocityRadps = 0.0}},
            });
        }
        for (wp::joint::JointId id : {wheels->frontLeft, wheels->frontRight,
                                      wheels->backLeft,  wheels->backRight}) {
            jointDrv.setTorqueEnable(id, on);
        }
    };
    mode.onChange([&](wp::mode::Mode old, wp::mode::Mode now) {
        std::cerr << "waypoint-core: mode " << wp::mode::name(old)
                  << " -> " << wp::mode::name(now) << "\n";
        publishMode(old, now);
        // Manual and Autonomous switch directly without a disarm in between,
        // so clear the target here: the old authority's last command must not
        // keep driving until the staleness watchdog catches it.
        if (drive) drive->setBodyTarget({});
        armDrive(now == wp::mode::Mode::Manual || now == wp::mode::Mode::Autonomous);
        if (now == wp::mode::Mode::Estop) {
            // The servo broker refuses module writes while estopped, so core is
            // the only path that can stop module wheel servos. Same latched
            // GOAL_SPEED reason as armDrive: zero first, then drop torque.
            for (wp::joint::JointId id : desc->moduleWheelBusIds()) {
                jointDrv.writeCommands({{id, {.velocityRadps = 0.0}}});
                jointDrv.setTorqueEnable(id, false);
            }
        }
    });
    // Initial snapshot so dashboards connecting after core boot learn the
    // current mode without waiting for a transition.
    publishMode(mode.current(), mode.current());

    nc.subscribe("waypoint." + args.roverID + ".infra.heartbeat",
        [&](const wp::nats::Message&) {
            wd.kick();
            mode.onHeartbeatRestored();
        });

    if (drive) {
        nc.subscribe("waypoint." + args.roverID + ".cmd.drive",
            [&](const wp::nats::Message& m) {
                if (mode.current() != wp::mode::Mode::Manual &&
                    mode.current() != wp::mode::Mode::Autonomous) return;
                waypoint::v1::DriveCommand cmd;
                if (!cmd.ParseFromString(m.payload)) return;
                drive->setBodyTarget({.vx = cmd.body_vx_mps(),
                                      .omegaZ = cmd.yaw_rate_radps()});
                driveSafety.onCommand();
            });
    }

    // cmd.arm is intentionally unbound: drive-only daemon. Module subscribers
    // own that subject.

    wp::rpc::SetModeHandler setMode(&nc, &mode, args.roverID);
    wp::rpc::EstopHandler   estop  (&nc, &mode, args.roverID);
    wp::rpc::RecoverHandler recover(&nc, &mode, args.roverID);
    wp::rpc::GetInfoHandler info   (&nc, args.roverID, kImageVersion);

    wp::servo::ServoControlServer servoCtl(
        &nc, bus.get(),
        [&mode] { return mode.current() == wp::mode::Mode::Estop; },
        args.roverID, desc->platformOwnedBusIds());

    std::unique_ptr<wp::sim::SimControlServer> simCtl;
    if (useSim) {
        simCtl = std::make_unique<wp::sim::SimControlServer>(
            &nc, args.roverID, simModel, simClock);
        // The bus owns the SimUart for the process lifetime; the raw pointer
        // feeds the malformed-frame counter into rpc.sim_info.
        simCtl->setMalformedSource(
            [simUartRaw] { return simUartRaw->malformedFrames(); });
    }

    wp::telemetry::DrivePublisher driveT(&nc, args.roverID);
    wp::telemetry::MotorPublisher motorT(&nc, args.roverID);
    wp::telemetry::PowerPublisher powerT(&nc, args.roverID);

    // infra.platform: announce the loaded descriptor at boot, re-announce on
    // a 5 s cadence (NATS core has no retention).
    wp::telemetry::PlatformPublisher platT(&nc, args.roverID, *desc);
    platT.publish();

    const std::string faultSubject = "waypoint." + args.roverID + ".event.fault";
    auto publishHeartbeatLost = [&]() {
        waypoint::v1::FaultEvent ev;
        ev.set_source("core");
        ev.set_severity(waypoint::v1::SEVERITY_FAULT);
        ev.set_code("heartbeat_lost");
        ev.set_message("agent heartbeat absent for >500ms");
        std::string out;
        ev.SerializeToString(&out);
        nc.publish(faultSubject, out.data(), out.size());
    };

    auto publishDriveStale = [&]() {
        waypoint::v1::FaultEvent ev;
        ev.set_source("core");
        ev.set_severity(waypoint::v1::SEVERITY_FAULT);
        ev.set_code("drive_command_stale");
        ev.set_message("cmd.drive stream stale for >300ms with nonzero target; drive target zeroed");
        std::string out;
        ev.SerializeToString(&out);
        nc.publish(faultSubject, out.data(), out.size());
    };

    // Phase 6: publish alert.raised on watchdog miss so the dashboard's
    // active-alerts panel reflects the fault. Deterministic id makes repeated
    // raises idempotent — the rover's local alerts store dedupes on it, so
    // consecutive misses don't create duplicate active rows.
    const std::string alertSubject = "waypoint." + args.roverID + ".alert.raised";
    const std::string watchdogAlertID = args.roverID + ":core.watchdog:heartbeat_lost";
    auto publishAlertRaised = [&]() {
        waypoint::v1::AlertRaised ev;
        ev.set_id(watchdogAlertID);
        ev.set_source("core.watchdog");
        ev.set_severity(waypoint::v1::SEVERITY_FAULT);
        ev.set_code("heartbeat_lost");
        ev.set_message("agent heartbeat absent for >500ms; entering safe mode");
        std::string out;
        ev.SerializeToString(&out);
        nc.publish(alertSubject, out.data(), out.size());
    };

    // Learn which descriptor-declared servos are on the bus. Drive control
    // stays on the wheel joints; any other servo found (arm/drill) gets
    // telemetry-only reads.
    std::vector<std::uint8_t> present =
        wp::servo::discoverServos(*bus, desc->allBusIds(), kScanPingTimeoutMs);
    std::vector<std::uint8_t> auxIDs;
    for (std::uint8_t id : present) {
        const bool isDriveWheel = wheels &&
            (id == wheels->frontLeft || id == wheels->frontRight ||
             id == wheels->backLeft  || id == wheels->backRight);
        if (!isDriveWheel) auxIDs.push_back(id);
    }
    std::cerr << "waypoint-core: bus telemetry — " << auxIDs.size()
              << " non-drive servo(s) discovered\n";
    // Mode lives in EEPROM; the driver reads first and only rewrites on
    // disagreement. Torque stays OFF here; armDrive() flips it on Manual.
    int modesOk = jointDrv.ensureModes(present);
    std::cerr << "waypoint-core: operating modes ensured (" << modesOk << "/"
              << present.size() << ")\n";
    // Belt and suspenders: some STS3215 clones boot with TORQUE_ENABLE=1, which
    // would let the idle PID loop drive the wheels before Manual is requested.
    armDrive(false);
    for (std::uint8_t id : auxIDs) bus->setTorqueEnable(id, false);

    wp::servo::AuxRotation auxRot(std::move(auxIDs), kAuxMaxFailures);
    wp::telemetry::ServoStateCache motorCache;

    auto next50 = clock->now();
    auto next20 = next50;
    auto next1  = next50;
    auto nextPlat = next50;
    while (!g_stop.load()) {
        auto now = clock->now();

        if (wd.tickAndCheck()) {
            std::cerr << "waypoint-core: heartbeat lost — dropping to safe\n";
            mode.onHeartbeatLost();
            if (drive) drive->setBodyTarget({});
            publishHeartbeatLost();
            publishAlertRaised();
        }

        if (drive && driveSafety.stale()) {
            auto t = drive->target();
            if (t.vx != 0.0 || t.omegaZ != 0.0) {
                std::cerr << "waypoint-core: cmd.drive stale, zeroing drive target\n";
                publishDriveStale();
            }
            drive->setBodyTarget({});
        }

        if (now >= next50) {
            // Advance the plant before the controller acts, for deterministic
            // step ordering under controlled time.
            if (simModel) simModel->step(0.02);
            if (drive) {
                drive->tick(0.02);
                // Body velocity reconstructed from measured wheel speeds (forward
                // kinematics is the average of L/R wheel linear velocities).
                auto m = drive->lastMeasured();
                double vL = 0.5 * (m.frontLeft + m.backLeft) * kin->wheelRadius_m;
                double vR = 0.5 * (m.frontRight + m.backRight) * kin->wheelRadius_m;
                double bodyVx = 0.5 * (vL + vR);
                double yawRate = (vR - vL) / kin->trackWidth_m;
                driveT.publish(bodyVx, yawRate);
            }
            next50 += 20ms;
        }
        if (now >= next20) {
            if (wheels) {
                for (std::uint8_t id : {wheels->frontLeft, wheels->frontRight,
                                        wheels->backLeft,  wheels->backRight}) {
                    if (auto state = bus->readState(id)) {
                        motorT.publish(id, *state);
                        motorCache.update(id, *state, now);
                    }
                }
            }
            // Non-drive servos, a bounded few per tick (round-robin), so their
            // reads can't starve the 50 Hz control loop.
            for (std::uint8_t id : auxRot.next(kAuxReadsPerTick)) {
                auto state = bus->readState(id);
                if (state) {
                    motorT.publish(id, *state);
                    motorCache.update(id, *state, now);
                }
                auxRot.recordResult(id, state.has_value());
            }
            next20 += 50ms;
        }
        if (now >= next1) {
            // Power from the per-servo telemetry cache: bus voltage (shared)
            // from the freshest servo, current summed across every servo read
            // recently (drive + arm + drill), so draw reflects the whole bus.
            double bus_v = motorCache.freshestVoltage(now, kPowerFreshWindow).value_or(0.0);
            double sum_i = motorCache.sumCurrentFresh(now, kPowerFreshWindow);
            powerT.publish(bus_v, sum_i);
            // Re-announce current mode so dashboards that connect mid-session
            // observe it within a second even if they missed the boot publish.
            publishMode(mode.current(), mode.current());
            next1 += 1s;
        }
        if (now >= nextPlat) {
            platT.publish();
            nextPlat += 5s;
        }
        clock->sleepFor(5ms);
        // Controlled-mode shutdown relies on SIGTERM interrupting the process,
        // the same as systemd stop semantics; the sleepFor above blocks until
        // the harness grants time, so re-check g_stop after it returns.
        if (g_stop.load()) break;
    }

    std::cerr << "waypoint-core: shutting down\n";
    if (simClock) simClock->stop();
    nc.stop();
    bus->close();
    google::protobuf::ShutdownProtobufLibrary();
    return 0;
}
