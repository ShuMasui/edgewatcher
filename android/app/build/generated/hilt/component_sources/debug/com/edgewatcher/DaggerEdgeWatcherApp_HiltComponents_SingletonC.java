package com.edgewatcher;

import android.app.Activity;
import android.app.Service;
import android.view.View;
import androidx.fragment.app.Fragment;
import androidx.lifecycle.SavedStateHandle;
import androidx.lifecycle.ViewModel;
import com.edgewatcher.di.AppModule_AlarmsFactory;
import com.edgewatcher.di.AppModule_ApiFactory;
import com.edgewatcher.di.AppModule_BufferFactory;
import com.edgewatcher.di.AppModule_CameraFactory;
import com.edgewatcher.di.AppModule_CameraGatewayFactory;
import com.edgewatcher.di.AppModule_CaptureObservationFactory;
import com.edgewatcher.di.AppModule_ClockFactory;
import com.edgewatcher.di.AppModule_CredentialStoreFactory;
import com.edgewatcher.di.AppModule_DatabaseFactory;
import com.edgewatcher.di.AppModule_DeviceInfoFactory;
import com.edgewatcher.di.AppModule_EncoderFactory;
import com.edgewatcher.di.AppModule_IdsFactory;
import com.edgewatcher.di.AppModule_JsonFactory;
import com.edgewatcher.di.AppModule_LocationFactory;
import com.edgewatcher.di.AppModule_LogoutDeviceFactory;
import com.edgewatcher.di.AppModule_OkHttpFactory;
import com.edgewatcher.di.AppModule_PairDeviceFactory;
import com.edgewatcher.di.AppModule_RefreshSessionFactory;
import com.edgewatcher.di.AppModule_ServiceFactory;
import com.edgewatcher.di.AppModule_UploadNextFactory;
import com.edgewatcher.domain.model.DeviceInfo;
import com.edgewatcher.domain.port.AlarmScheduler;
import com.edgewatcher.domain.port.CameraGateway;
import com.edgewatcher.domain.port.Clock;
import com.edgewatcher.domain.port.CredentialStore;
import com.edgewatcher.domain.port.IdGenerator;
import com.edgewatcher.domain.port.JpegEncoder;
import com.edgewatcher.domain.port.LocationGateway;
import com.edgewatcher.domain.port.ObservationApi;
import com.edgewatcher.domain.port.ObservationBuffer;
import com.edgewatcher.infrastructure.api.EdgeWatcherService;
import com.edgewatcher.infrastructure.camera.CameraXGateway;
import com.edgewatcher.infrastructure.db.ObservationDatabase;
import com.edgewatcher.infrastructure.service.BootReceiver;
import com.edgewatcher.infrastructure.service.BootReceiver_MembersInjector;
import com.edgewatcher.infrastructure.service.ObservationService;
import com.edgewatcher.infrastructure.service.ObservationService_MembersInjector;
import com.edgewatcher.presentation.MainActivity;
import com.edgewatcher.presentation.MainActivity_MembersInjector;
import com.edgewatcher.presentation.pairing.PairingViewModel;
import com.edgewatcher.presentation.pairing.PairingViewModel_HiltModules;
import com.edgewatcher.presentation.pairing.PairingViewModel_HiltModules_BindsModule_Binds_LazyMapKey;
import com.edgewatcher.presentation.pairing.PairingViewModel_HiltModules_KeyModule_Provide_LazyMapKey;
import com.edgewatcher.presentation.running.RunningViewModel;
import com.edgewatcher.presentation.running.RunningViewModel_HiltModules;
import com.edgewatcher.presentation.running.RunningViewModel_HiltModules_BindsModule_Binds_LazyMapKey;
import com.edgewatcher.presentation.running.RunningViewModel_HiltModules_KeyModule_Provide_LazyMapKey;
import com.edgewatcher.usecase.CaptureObservationUseCase;
import com.edgewatcher.usecase.LogoutDeviceUseCase;
import com.edgewatcher.usecase.PairDeviceUseCase;
import com.edgewatcher.usecase.RefreshSessionUseCase;
import com.edgewatcher.usecase.UploadNextObservationUseCase;
import dagger.hilt.android.ActivityRetainedLifecycle;
import dagger.hilt.android.ViewModelLifecycle;
import dagger.hilt.android.internal.builders.ActivityComponentBuilder;
import dagger.hilt.android.internal.builders.ActivityRetainedComponentBuilder;
import dagger.hilt.android.internal.builders.FragmentComponentBuilder;
import dagger.hilt.android.internal.builders.ServiceComponentBuilder;
import dagger.hilt.android.internal.builders.ViewComponentBuilder;
import dagger.hilt.android.internal.builders.ViewModelComponentBuilder;
import dagger.hilt.android.internal.builders.ViewWithFragmentComponentBuilder;
import dagger.hilt.android.internal.lifecycle.DefaultViewModelFactories;
import dagger.hilt.android.internal.lifecycle.DefaultViewModelFactories_InternalFactoryFactory_Factory;
import dagger.hilt.android.internal.managers.ActivityRetainedComponentManager_LifecycleModule_ProvideActivityRetainedLifecycleFactory;
import dagger.hilt.android.internal.managers.SavedStateHandleHolder;
import dagger.hilt.android.internal.modules.ApplicationContextModule;
import dagger.hilt.android.internal.modules.ApplicationContextModule_ProvideContextFactory;
import dagger.internal.DaggerGenerated;
import dagger.internal.DoubleCheck;
import dagger.internal.LazyClassKeyMap;
import dagger.internal.MapBuilder;
import dagger.internal.Preconditions;
import dagger.internal.Provider;
import java.util.Collections;
import java.util.Map;
import java.util.Set;
import javax.annotation.processing.Generated;
import kotlinx.serialization.json.Json;
import okhttp3.OkHttpClient;

@DaggerGenerated
@Generated(
    value = "dagger.internal.codegen.ComponentProcessor",
    comments = "https://dagger.dev"
)
@SuppressWarnings({
    "unchecked",
    "rawtypes",
    "KotlinInternal",
    "KotlinInternalInJava",
    "cast",
    "deprecation",
    "nullness:initialization.field.uninitialized"
})
public final class DaggerEdgeWatcherApp_HiltComponents_SingletonC {
  private DaggerEdgeWatcherApp_HiltComponents_SingletonC() {
  }

  public static Builder builder() {
    return new Builder();
  }

  public static final class Builder {
    private ApplicationContextModule applicationContextModule;

    private Builder() {
    }

    public Builder applicationContextModule(ApplicationContextModule applicationContextModule) {
      this.applicationContextModule = Preconditions.checkNotNull(applicationContextModule);
      return this;
    }

    public EdgeWatcherApp_HiltComponents.SingletonC build() {
      Preconditions.checkBuilderRequirement(applicationContextModule, ApplicationContextModule.class);
      return new SingletonCImpl(applicationContextModule);
    }
  }

  private static final class ActivityRetainedCBuilder implements EdgeWatcherApp_HiltComponents.ActivityRetainedC.Builder {
    private final SingletonCImpl singletonCImpl;

    private SavedStateHandleHolder savedStateHandleHolder;

    private ActivityRetainedCBuilder(SingletonCImpl singletonCImpl) {
      this.singletonCImpl = singletonCImpl;
    }

    @Override
    public ActivityRetainedCBuilder savedStateHandleHolder(
        SavedStateHandleHolder savedStateHandleHolder) {
      this.savedStateHandleHolder = Preconditions.checkNotNull(savedStateHandleHolder);
      return this;
    }

    @Override
    public EdgeWatcherApp_HiltComponents.ActivityRetainedC build() {
      Preconditions.checkBuilderRequirement(savedStateHandleHolder, SavedStateHandleHolder.class);
      return new ActivityRetainedCImpl(singletonCImpl, savedStateHandleHolder);
    }
  }

  private static final class ActivityCBuilder implements EdgeWatcherApp_HiltComponents.ActivityC.Builder {
    private final SingletonCImpl singletonCImpl;

    private final ActivityRetainedCImpl activityRetainedCImpl;

    private Activity activity;

    private ActivityCBuilder(SingletonCImpl singletonCImpl,
        ActivityRetainedCImpl activityRetainedCImpl) {
      this.singletonCImpl = singletonCImpl;
      this.activityRetainedCImpl = activityRetainedCImpl;
    }

    @Override
    public ActivityCBuilder activity(Activity activity) {
      this.activity = Preconditions.checkNotNull(activity);
      return this;
    }

    @Override
    public EdgeWatcherApp_HiltComponents.ActivityC build() {
      Preconditions.checkBuilderRequirement(activity, Activity.class);
      return new ActivityCImpl(singletonCImpl, activityRetainedCImpl, activity);
    }
  }

  private static final class FragmentCBuilder implements EdgeWatcherApp_HiltComponents.FragmentC.Builder {
    private final SingletonCImpl singletonCImpl;

    private final ActivityRetainedCImpl activityRetainedCImpl;

    private final ActivityCImpl activityCImpl;

    private Fragment fragment;

    private FragmentCBuilder(SingletonCImpl singletonCImpl,
        ActivityRetainedCImpl activityRetainedCImpl, ActivityCImpl activityCImpl) {
      this.singletonCImpl = singletonCImpl;
      this.activityRetainedCImpl = activityRetainedCImpl;
      this.activityCImpl = activityCImpl;
    }

    @Override
    public FragmentCBuilder fragment(Fragment fragment) {
      this.fragment = Preconditions.checkNotNull(fragment);
      return this;
    }

    @Override
    public EdgeWatcherApp_HiltComponents.FragmentC build() {
      Preconditions.checkBuilderRequirement(fragment, Fragment.class);
      return new FragmentCImpl(singletonCImpl, activityRetainedCImpl, activityCImpl, fragment);
    }
  }

  private static final class ViewWithFragmentCBuilder implements EdgeWatcherApp_HiltComponents.ViewWithFragmentC.Builder {
    private final SingletonCImpl singletonCImpl;

    private final ActivityRetainedCImpl activityRetainedCImpl;

    private final ActivityCImpl activityCImpl;

    private final FragmentCImpl fragmentCImpl;

    private View view;

    private ViewWithFragmentCBuilder(SingletonCImpl singletonCImpl,
        ActivityRetainedCImpl activityRetainedCImpl, ActivityCImpl activityCImpl,
        FragmentCImpl fragmentCImpl) {
      this.singletonCImpl = singletonCImpl;
      this.activityRetainedCImpl = activityRetainedCImpl;
      this.activityCImpl = activityCImpl;
      this.fragmentCImpl = fragmentCImpl;
    }

    @Override
    public ViewWithFragmentCBuilder view(View view) {
      this.view = Preconditions.checkNotNull(view);
      return this;
    }

    @Override
    public EdgeWatcherApp_HiltComponents.ViewWithFragmentC build() {
      Preconditions.checkBuilderRequirement(view, View.class);
      return new ViewWithFragmentCImpl(singletonCImpl, activityRetainedCImpl, activityCImpl, fragmentCImpl, view);
    }
  }

  private static final class ViewCBuilder implements EdgeWatcherApp_HiltComponents.ViewC.Builder {
    private final SingletonCImpl singletonCImpl;

    private final ActivityRetainedCImpl activityRetainedCImpl;

    private final ActivityCImpl activityCImpl;

    private View view;

    private ViewCBuilder(SingletonCImpl singletonCImpl, ActivityRetainedCImpl activityRetainedCImpl,
        ActivityCImpl activityCImpl) {
      this.singletonCImpl = singletonCImpl;
      this.activityRetainedCImpl = activityRetainedCImpl;
      this.activityCImpl = activityCImpl;
    }

    @Override
    public ViewCBuilder view(View view) {
      this.view = Preconditions.checkNotNull(view);
      return this;
    }

    @Override
    public EdgeWatcherApp_HiltComponents.ViewC build() {
      Preconditions.checkBuilderRequirement(view, View.class);
      return new ViewCImpl(singletonCImpl, activityRetainedCImpl, activityCImpl, view);
    }
  }

  private static final class ViewModelCBuilder implements EdgeWatcherApp_HiltComponents.ViewModelC.Builder {
    private final SingletonCImpl singletonCImpl;

    private final ActivityRetainedCImpl activityRetainedCImpl;

    private SavedStateHandle savedStateHandle;

    private ViewModelLifecycle viewModelLifecycle;

    private ViewModelCBuilder(SingletonCImpl singletonCImpl,
        ActivityRetainedCImpl activityRetainedCImpl) {
      this.singletonCImpl = singletonCImpl;
      this.activityRetainedCImpl = activityRetainedCImpl;
    }

    @Override
    public ViewModelCBuilder savedStateHandle(SavedStateHandle handle) {
      this.savedStateHandle = Preconditions.checkNotNull(handle);
      return this;
    }

    @Override
    public ViewModelCBuilder viewModelLifecycle(ViewModelLifecycle viewModelLifecycle) {
      this.viewModelLifecycle = Preconditions.checkNotNull(viewModelLifecycle);
      return this;
    }

    @Override
    public EdgeWatcherApp_HiltComponents.ViewModelC build() {
      Preconditions.checkBuilderRequirement(savedStateHandle, SavedStateHandle.class);
      Preconditions.checkBuilderRequirement(viewModelLifecycle, ViewModelLifecycle.class);
      return new ViewModelCImpl(singletonCImpl, activityRetainedCImpl, savedStateHandle, viewModelLifecycle);
    }
  }

  private static final class ServiceCBuilder implements EdgeWatcherApp_HiltComponents.ServiceC.Builder {
    private final SingletonCImpl singletonCImpl;

    private Service service;

    private ServiceCBuilder(SingletonCImpl singletonCImpl) {
      this.singletonCImpl = singletonCImpl;
    }

    @Override
    public ServiceCBuilder service(Service service) {
      this.service = Preconditions.checkNotNull(service);
      return this;
    }

    @Override
    public EdgeWatcherApp_HiltComponents.ServiceC build() {
      Preconditions.checkBuilderRequirement(service, Service.class);
      return new ServiceCImpl(singletonCImpl, service);
    }
  }

  private static final class ViewWithFragmentCImpl extends EdgeWatcherApp_HiltComponents.ViewWithFragmentC {
    private final SingletonCImpl singletonCImpl;

    private final ActivityRetainedCImpl activityRetainedCImpl;

    private final ActivityCImpl activityCImpl;

    private final FragmentCImpl fragmentCImpl;

    private final ViewWithFragmentCImpl viewWithFragmentCImpl = this;

    ViewWithFragmentCImpl(SingletonCImpl singletonCImpl,
        ActivityRetainedCImpl activityRetainedCImpl, ActivityCImpl activityCImpl,
        FragmentCImpl fragmentCImpl, View viewParam) {
      this.singletonCImpl = singletonCImpl;
      this.activityRetainedCImpl = activityRetainedCImpl;
      this.activityCImpl = activityCImpl;
      this.fragmentCImpl = fragmentCImpl;


    }
  }

  private static final class FragmentCImpl extends EdgeWatcherApp_HiltComponents.FragmentC {
    private final SingletonCImpl singletonCImpl;

    private final ActivityRetainedCImpl activityRetainedCImpl;

    private final ActivityCImpl activityCImpl;

    private final FragmentCImpl fragmentCImpl = this;

    FragmentCImpl(SingletonCImpl singletonCImpl, ActivityRetainedCImpl activityRetainedCImpl,
        ActivityCImpl activityCImpl, Fragment fragmentParam) {
      this.singletonCImpl = singletonCImpl;
      this.activityRetainedCImpl = activityRetainedCImpl;
      this.activityCImpl = activityCImpl;


    }

    @Override
    public DefaultViewModelFactories.InternalFactoryFactory getHiltInternalFactoryFactory() {
      return activityCImpl.getHiltInternalFactoryFactory();
    }

    @Override
    public ViewWithFragmentComponentBuilder viewWithFragmentComponentBuilder() {
      return new ViewWithFragmentCBuilder(singletonCImpl, activityRetainedCImpl, activityCImpl, fragmentCImpl);
    }
  }

  private static final class ViewCImpl extends EdgeWatcherApp_HiltComponents.ViewC {
    private final SingletonCImpl singletonCImpl;

    private final ActivityRetainedCImpl activityRetainedCImpl;

    private final ActivityCImpl activityCImpl;

    private final ViewCImpl viewCImpl = this;

    ViewCImpl(SingletonCImpl singletonCImpl, ActivityRetainedCImpl activityRetainedCImpl,
        ActivityCImpl activityCImpl, View viewParam) {
      this.singletonCImpl = singletonCImpl;
      this.activityRetainedCImpl = activityRetainedCImpl;
      this.activityCImpl = activityCImpl;


    }
  }

  private static final class ActivityCImpl extends EdgeWatcherApp_HiltComponents.ActivityC {
    private final SingletonCImpl singletonCImpl;

    private final ActivityRetainedCImpl activityRetainedCImpl;

    private final ActivityCImpl activityCImpl = this;

    ActivityCImpl(SingletonCImpl singletonCImpl, ActivityRetainedCImpl activityRetainedCImpl,
        Activity activityParam) {
      this.singletonCImpl = singletonCImpl;
      this.activityRetainedCImpl = activityRetainedCImpl;


    }

    @Override
    public void injectMainActivity(MainActivity mainActivity) {
      injectMainActivity2(mainActivity);
    }

    @Override
    public DefaultViewModelFactories.InternalFactoryFactory getHiltInternalFactoryFactory() {
      return DefaultViewModelFactories_InternalFactoryFactory_Factory.newInstance(getViewModelKeys(), new ViewModelCBuilder(singletonCImpl, activityRetainedCImpl));
    }

    @Override
    public Map<Class<?>, Boolean> getViewModelKeys() {
      return LazyClassKeyMap.<Boolean>of(MapBuilder.<String, Boolean>newMapBuilder(2).put(PairingViewModel_HiltModules_KeyModule_Provide_LazyMapKey.lazyClassKeyName, PairingViewModel_HiltModules.KeyModule.provide()).put(RunningViewModel_HiltModules_KeyModule_Provide_LazyMapKey.lazyClassKeyName, RunningViewModel_HiltModules.KeyModule.provide()).build());
    }

    @Override
    public ViewModelComponentBuilder getViewModelComponentBuilder() {
      return new ViewModelCBuilder(singletonCImpl, activityRetainedCImpl);
    }

    @Override
    public FragmentComponentBuilder fragmentComponentBuilder() {
      return new FragmentCBuilder(singletonCImpl, activityRetainedCImpl, activityCImpl);
    }

    @Override
    public ViewComponentBuilder viewComponentBuilder() {
      return new ViewCBuilder(singletonCImpl, activityRetainedCImpl, activityCImpl);
    }

    private MainActivity injectMainActivity2(MainActivity instance) {
      MainActivity_MembersInjector.injectStore(instance, singletonCImpl.credentialStoreProvider.get());
      return instance;
    }
  }

  private static final class ViewModelCImpl extends EdgeWatcherApp_HiltComponents.ViewModelC {
    private final SingletonCImpl singletonCImpl;

    private final ActivityRetainedCImpl activityRetainedCImpl;

    private final ViewModelCImpl viewModelCImpl = this;

    Provider<PairingViewModel> pairingViewModelProvider;

    Provider<RunningViewModel> runningViewModelProvider;

    ViewModelCImpl(SingletonCImpl singletonCImpl, ActivityRetainedCImpl activityRetainedCImpl,
        SavedStateHandle savedStateHandleParam, ViewModelLifecycle viewModelLifecycleParam) {
      this.singletonCImpl = singletonCImpl;
      this.activityRetainedCImpl = activityRetainedCImpl;

      initialize(savedStateHandleParam, viewModelLifecycleParam);

    }

    @SuppressWarnings("unchecked")
    private void initialize(final SavedStateHandle savedStateHandleParam,
        final ViewModelLifecycle viewModelLifecycleParam) {
      this.pairingViewModelProvider = new SwitchingProvider<>(singletonCImpl, activityRetainedCImpl, viewModelCImpl, 0);
      this.runningViewModelProvider = new SwitchingProvider<>(singletonCImpl, activityRetainedCImpl, viewModelCImpl, 1);
    }

    @Override
    public Map<Class<?>, javax.inject.Provider<ViewModel>> getHiltViewModelMap() {
      return LazyClassKeyMap.<javax.inject.Provider<ViewModel>>of(MapBuilder.<String, javax.inject.Provider<ViewModel>>newMapBuilder(2).put(PairingViewModel_HiltModules_BindsModule_Binds_LazyMapKey.lazyClassKeyName, ((Provider) (pairingViewModelProvider))).put(RunningViewModel_HiltModules_BindsModule_Binds_LazyMapKey.lazyClassKeyName, ((Provider) (runningViewModelProvider))).build());
    }

    @Override
    public Map<Class<?>, Object> getHiltViewModelAssistedMap() {
      return Collections.<Class<?>, Object>emptyMap();
    }

    private static final class SwitchingProvider<T> implements Provider<T> {
      private final SingletonCImpl singletonCImpl;

      private final ActivityRetainedCImpl activityRetainedCImpl;

      private final ViewModelCImpl viewModelCImpl;

      private final int id;

      SwitchingProvider(SingletonCImpl singletonCImpl, ActivityRetainedCImpl activityRetainedCImpl,
          ViewModelCImpl viewModelCImpl, int id) {
        this.singletonCImpl = singletonCImpl;
        this.activityRetainedCImpl = activityRetainedCImpl;
        this.viewModelCImpl = viewModelCImpl;
        this.id = id;
      }

      @Override
      @SuppressWarnings("unchecked")
      public T get() {
        switch (id) {
          case 0: // com.edgewatcher.presentation.pairing.PairingViewModel
          return (T) new PairingViewModel(singletonCImpl.pairDeviceUseCase(), singletonCImpl.deviceInfoProvider.get());

          case 1: // com.edgewatcher.presentation.running.RunningViewModel
          return (T) new RunningViewModel(singletonCImpl.logoutDeviceUseCase());

          default: throw new AssertionError(id);
        }
      }
    }
  }

  private static final class ActivityRetainedCImpl extends EdgeWatcherApp_HiltComponents.ActivityRetainedC {
    private final SingletonCImpl singletonCImpl;

    private final ActivityRetainedCImpl activityRetainedCImpl = this;

    Provider<ActivityRetainedLifecycle> provideActivityRetainedLifecycleProvider;

    ActivityRetainedCImpl(SingletonCImpl singletonCImpl,
        SavedStateHandleHolder savedStateHandleHolderParam) {
      this.singletonCImpl = singletonCImpl;

      initialize(savedStateHandleHolderParam);

    }

    @SuppressWarnings("unchecked")
    private void initialize(final SavedStateHandleHolder savedStateHandleHolderParam) {
      this.provideActivityRetainedLifecycleProvider = DoubleCheck.provider(new SwitchingProvider<ActivityRetainedLifecycle>(singletonCImpl, activityRetainedCImpl, 0));
    }

    @Override
    public ActivityComponentBuilder activityComponentBuilder() {
      return new ActivityCBuilder(singletonCImpl, activityRetainedCImpl);
    }

    @Override
    public ActivityRetainedLifecycle getActivityRetainedLifecycle() {
      return provideActivityRetainedLifecycleProvider.get();
    }

    private static final class SwitchingProvider<T> implements Provider<T> {
      private final SingletonCImpl singletonCImpl;

      private final ActivityRetainedCImpl activityRetainedCImpl;

      private final int id;

      SwitchingProvider(SingletonCImpl singletonCImpl, ActivityRetainedCImpl activityRetainedCImpl,
          int id) {
        this.singletonCImpl = singletonCImpl;
        this.activityRetainedCImpl = activityRetainedCImpl;
        this.id = id;
      }

      @Override
      @SuppressWarnings("unchecked")
      public T get() {
        switch (id) {
          case 0: // dagger.hilt.android.ActivityRetainedLifecycle
          return (T) ActivityRetainedComponentManager_LifecycleModule_ProvideActivityRetainedLifecycleFactory.provideActivityRetainedLifecycle();

          default: throw new AssertionError(id);
        }
      }
    }
  }

  private static final class ServiceCImpl extends EdgeWatcherApp_HiltComponents.ServiceC {
    private final SingletonCImpl singletonCImpl;

    private final ServiceCImpl serviceCImpl = this;

    ServiceCImpl(SingletonCImpl singletonCImpl, Service serviceParam) {
      this.singletonCImpl = singletonCImpl;


    }

    @Override
    public void injectObservationService(ObservationService observationService) {
      injectObservationService2(observationService);
    }

    private ObservationService injectObservationService2(ObservationService instance) {
      ObservationService_MembersInjector.injectCapture(instance, singletonCImpl.captureObservationUseCase());
      ObservationService_MembersInjector.injectUploadNext(instance, singletonCImpl.uploadNextObservationUseCase());
      ObservationService_MembersInjector.injectCamera(instance, singletonCImpl.cameraGatewayProvider.get());
      ObservationService_MembersInjector.injectAlarms(instance, singletonCImpl.alarmsProvider.get());
      ObservationService_MembersInjector.injectStore(instance, singletonCImpl.credentialStoreProvider.get());
      ObservationService_MembersInjector.injectClock(instance, singletonCImpl.clockProvider.get());
      ObservationService_MembersInjector.injectBuffer(instance, singletonCImpl.bufferProvider.get());
      return instance;
    }
  }

  private static final class SingletonCImpl extends EdgeWatcherApp_HiltComponents.SingletonC {
    private final ApplicationContextModule applicationContextModule;

    private final SingletonCImpl singletonCImpl = this;

    Provider<CredentialStore> credentialStoreProvider;

    Provider<OkHttpClient> okHttpProvider;

    Provider<Json> jsonProvider;

    Provider<EdgeWatcherService> serviceProvider;

    Provider<ObservationApi> apiProvider;

    Provider<DeviceInfo> deviceInfoProvider;

    Provider<ObservationDatabase> databaseProvider;

    Provider<ObservationBuffer> bufferProvider;

    Provider<Clock> clockProvider;

    Provider<IdGenerator> idsProvider;

    Provider<CameraXGateway> cameraGatewayProvider;

    Provider<CameraGateway> cameraProvider;

    Provider<JpegEncoder> encoderProvider;

    Provider<LocationGateway> locationProvider;

    Provider<AlarmScheduler> alarmsProvider;

    SingletonCImpl(ApplicationContextModule applicationContextModuleParam) {
      this.applicationContextModule = applicationContextModuleParam;
      initialize(applicationContextModuleParam);

    }

    PairDeviceUseCase pairDeviceUseCase() {
      return AppModule_PairDeviceFactory.pairDevice(apiProvider.get(), credentialStoreProvider.get());
    }

    LogoutDeviceUseCase logoutDeviceUseCase() {
      return AppModule_LogoutDeviceFactory.logoutDevice(apiProvider.get(), credentialStoreProvider.get(), bufferProvider.get());
    }

    CaptureObservationUseCase captureObservationUseCase() {
      return AppModule_CaptureObservationFactory.captureObservation(clockProvider.get(), idsProvider.get(), cameraProvider.get(), encoderProvider.get(), locationProvider.get(), bufferProvider.get());
    }

    RefreshSessionUseCase refreshSessionUseCase() {
      return AppModule_RefreshSessionFactory.refreshSession(apiProvider.get(), credentialStoreProvider.get(), bufferProvider.get());
    }

    UploadNextObservationUseCase uploadNextObservationUseCase() {
      return AppModule_UploadNextFactory.uploadNext(apiProvider.get(), credentialStoreProvider.get(), bufferProvider.get(), clockProvider.get(), refreshSessionUseCase());
    }

    @SuppressWarnings("unchecked")
    private void initialize(final ApplicationContextModule applicationContextModuleParam) {
      this.credentialStoreProvider = DoubleCheck.provider(new SwitchingProvider<CredentialStore>(singletonCImpl, 0));
      this.okHttpProvider = DoubleCheck.provider(new SwitchingProvider<OkHttpClient>(singletonCImpl, 3));
      this.jsonProvider = DoubleCheck.provider(new SwitchingProvider<Json>(singletonCImpl, 4));
      this.serviceProvider = DoubleCheck.provider(new SwitchingProvider<EdgeWatcherService>(singletonCImpl, 2));
      this.apiProvider = DoubleCheck.provider(new SwitchingProvider<ObservationApi>(singletonCImpl, 1));
      this.deviceInfoProvider = DoubleCheck.provider(new SwitchingProvider<DeviceInfo>(singletonCImpl, 5));
      this.databaseProvider = DoubleCheck.provider(new SwitchingProvider<ObservationDatabase>(singletonCImpl, 7));
      this.bufferProvider = DoubleCheck.provider(new SwitchingProvider<ObservationBuffer>(singletonCImpl, 6));
      this.clockProvider = DoubleCheck.provider(new SwitchingProvider<Clock>(singletonCImpl, 8));
      this.idsProvider = DoubleCheck.provider(new SwitchingProvider<IdGenerator>(singletonCImpl, 9));
      this.cameraGatewayProvider = DoubleCheck.provider(new SwitchingProvider<CameraXGateway>(singletonCImpl, 11));
      this.cameraProvider = DoubleCheck.provider(new SwitchingProvider<CameraGateway>(singletonCImpl, 10));
      this.encoderProvider = DoubleCheck.provider(new SwitchingProvider<JpegEncoder>(singletonCImpl, 12));
      this.locationProvider = DoubleCheck.provider(new SwitchingProvider<LocationGateway>(singletonCImpl, 13));
      this.alarmsProvider = DoubleCheck.provider(new SwitchingProvider<AlarmScheduler>(singletonCImpl, 14));
    }

    @Override
    public void injectEdgeWatcherApp(EdgeWatcherApp edgeWatcherApp) {
    }

    @Override
    public void injectBootReceiver(BootReceiver bootReceiver) {
      injectBootReceiver2(bootReceiver);
    }

    @Override
    public Set<Boolean> getDisableFragmentGetContextFix() {
      return Collections.<Boolean>emptySet();
    }

    @Override
    public ActivityRetainedComponentBuilder retainedComponentBuilder() {
      return new ActivityRetainedCBuilder(singletonCImpl);
    }

    @Override
    public ServiceComponentBuilder serviceComponentBuilder() {
      return new ServiceCBuilder(singletonCImpl);
    }

    private BootReceiver injectBootReceiver2(BootReceiver instance) {
      BootReceiver_MembersInjector.injectStore(instance, credentialStoreProvider.get());
      return instance;
    }

    private static final class SwitchingProvider<T> implements Provider<T> {
      private final SingletonCImpl singletonCImpl;

      private final int id;

      SwitchingProvider(SingletonCImpl singletonCImpl, int id) {
        this.singletonCImpl = singletonCImpl;
        this.id = id;
      }

      @Override
      @SuppressWarnings("unchecked")
      public T get() {
        switch (id) {
          case 0: // com.edgewatcher.domain.port.CredentialStore
          return (T) AppModule_CredentialStoreFactory.credentialStore(ApplicationContextModule_ProvideContextFactory.provideContext(singletonCImpl.applicationContextModule));

          case 1: // com.edgewatcher.domain.port.ObservationApi
          return (T) AppModule_ApiFactory.api(singletonCImpl.serviceProvider.get(), singletonCImpl.jsonProvider.get());

          case 2: // com.edgewatcher.infrastructure.api.EdgeWatcherService
          return (T) AppModule_ServiceFactory.service(singletonCImpl.okHttpProvider.get(), singletonCImpl.jsonProvider.get());

          case 3: // okhttp3.OkHttpClient
          return (T) AppModule_OkHttpFactory.okHttp();

          case 4: // kotlinx.serialization.json.Json
          return (T) AppModule_JsonFactory.json();

          case 5: // com.edgewatcher.domain.model.DeviceInfo
          return (T) AppModule_DeviceInfoFactory.deviceInfo();

          case 6: // com.edgewatcher.domain.port.ObservationBuffer
          return (T) AppModule_BufferFactory.buffer(ApplicationContextModule_ProvideContextFactory.provideContext(singletonCImpl.applicationContextModule), singletonCImpl.databaseProvider.get());

          case 7: // com.edgewatcher.infrastructure.db.ObservationDatabase
          return (T) AppModule_DatabaseFactory.database(ApplicationContextModule_ProvideContextFactory.provideContext(singletonCImpl.applicationContextModule));

          case 8: // com.edgewatcher.domain.port.Clock
          return (T) AppModule_ClockFactory.clock();

          case 9: // com.edgewatcher.domain.port.IdGenerator
          return (T) AppModule_IdsFactory.ids();

          case 10: // com.edgewatcher.domain.port.CameraGateway
          return (T) AppModule_CameraFactory.camera(singletonCImpl.cameraGatewayProvider.get());

          case 11: // com.edgewatcher.infrastructure.camera.CameraXGateway
          return (T) AppModule_CameraGatewayFactory.cameraGateway(ApplicationContextModule_ProvideContextFactory.provideContext(singletonCImpl.applicationContextModule));

          case 12: // com.edgewatcher.domain.port.JpegEncoder
          return (T) AppModule_EncoderFactory.encoder();

          case 13: // com.edgewatcher.domain.port.LocationGateway
          return (T) AppModule_LocationFactory.location(ApplicationContextModule_ProvideContextFactory.provideContext(singletonCImpl.applicationContextModule));

          case 14: // com.edgewatcher.domain.port.AlarmScheduler
          return (T) AppModule_AlarmsFactory.alarms(ApplicationContextModule_ProvideContextFactory.provideContext(singletonCImpl.applicationContextModule));

          default: throw new AssertionError(id);
        }
      }
    }
  }
}
